// Stress test workflow migration Pathgres.
//
// Beban dinaikkan bertahap dari normal ke stress dan peak, lalu diturunkan
// kembali ke normal (recovery) untuk melihat apakah sistem pulih.
//
// Skenario (berjalan bersamaan):
//   reads   : riwayat (hide rollback), daftar migration, inbox approval, board.
//   workflow: create -> validate -> submit -> approve -> polling sampai worker
//             selesai. Setiap VU memakai salah satu DB_IDS, sehingga beberapa VU
//             berebut database yang sama sementara database lain diproses paralel.
//
// Variabel lingkungan:
//   BASE_URL   default http://localhost:8080
//   USERNAME, PASSWORD  user dengan akses View + Edit ke semua DB_IDS dan leader
//                       satu-satunya group yang memiliki akses ke database itu
//   DB_IDS     id koneksi target, dipisah koma (misalnya 3,4,5)
//   STEP       lama tiap tahap tahan beban (default 1m)
//   READ_RATE  request baca per detik pada beban normal (default 50)
//   WORKFLOW_VUS  VU workflow pada beban normal (default 4)
import http from 'k6/http';
import { check, fail, sleep } from 'k6';
import { Counter, Trend } from 'k6/metrics';

const BASE = __ENV.BASE_URL || 'http://localhost:8080';
const DBS = (__ENV.DB_IDS || __ENV.DB_ID || fail('DB_IDS wajib diisi')).split(',').map((s) => s.trim()).filter(Boolean);
const STEP = __ENV.STEP || '1m';
const READ_RATE = Number(__ENV.READ_RATE || 50);
const WORKFLOW_VUS = Number(__ENV.WORKFLOW_VUS || 4);
const RAMP = '20s';

// Tingkat beban: pengali dari beban normal. Urutan = urutan tahap.
const LEVELS = [
  { name: 'normal', factor: 1 },
  { name: 'stress', factor: 2 },
  { name: 'peak', factor: 4 },
  { name: 'recovery', factor: 1 },
];

function seconds(d) {
  const m = /^(\d+)(s|m)$/.exec(d);
  if (!m) fail(`durasi tidak valid: ${d}`);
  return Number(m[1]) * (m[2] === 'm' ? 60 : 1);
}

// Setiap tingkat = ramp ke target lalu tahan STEP. Batas waktu dipakai untuk
// memberi tag "level" pada setiap request agar hasil bisa dibaca per tingkat.
const STEP_S = seconds(STEP);
const RAMP_S = seconds(RAMP);
const BOUNDS = LEVELS.map((l, i) => ({ name: l.name, end: (i + 1) * (RAMP_S + STEP_S) }));

function stages(base) {
  return LEVELS.flatMap((l) => [
    { duration: RAMP, target: Math.max(1, Math.round(base * l.factor)) },
    { duration: STEP, target: Math.max(1, Math.round(base * l.factor)) },
  ]);
}

const queueWait = new Trend('job_queue_wait_ms', true);
const jobDuration = new Trend('job_run_ms', true);
const approveToDone = new Trend('job_approve_to_done_ms', true);
const jobsDone = new Counter('jobs_completed');
const workflowFailed = new Counter('workflow_failed');
const pollTimeout = new Counter('workflow_poll_timeout');

const thresholds = {
  'http_req_failed{scenario:reads}': ['rate<0.01'],
  'http_req_duration{scenario:reads}': ['p(95)<300'],
  'http_req_duration{name:approve}': ['p(95)<500'],
  workflow_failed: ['count==0'],
};
// Submetric per tingkat beban hanya muncul di ringkasan bila punya threshold;
// threshold longgar ini dipakai untuk menampilkan angka per tingkat.
for (const l of LEVELS) {
  thresholds[`http_req_duration{scenario:reads,level:${l.name}}`] = ['p(95)<1000'];
  thresholds[`http_req_failed{level:${l.name}}`] = ['rate<0.05'];
  thresholds[`http_reqs{level:${l.name}}`] = ['count>0'];
  thresholds[`job_queue_wait_ms{level:${l.name}}`] = ['p(95)<30000'];
  thresholds[`job_approve_to_done_ms{level:${l.name}}`] = ['p(95)<30000'];
  thresholds[`jobs_completed{level:${l.name}}`] = ['count>=0'];
}

export const options = {
  scenarios: {
    reads: {
      executor: 'ramping-arrival-rate',
      exec: 'reads',
      startRate: 1,
      timeUnit: '1s',
      stages: stages(READ_RATE),
      preAllocatedVUs: 30,
      maxVUs: 300,
    },
    workflow: {
      executor: 'ramping-vus',
      exec: 'workflow',
      startVUs: 1,
      stages: stages(WORKFLOW_VUS),
      gracefulRampDown: '30s',
    },
  },
  thresholds,
  summaryTrendStats: ['avg', 'p(50)', 'p(95)', 'max'],
};

// Login sekali; cookie sesi dan token CSRF dibagikan ke semua VU.
export function setup() {
  const res = http.post(`${BASE}/login`, { username: __ENV.USERNAME, password: __ENV.PASSWORD }, { redirects: 0 });
  const cookie = res.cookies.pathgres_session;
  if (!cookie || cookie.length === 0) fail(`login gagal: status ${res.status}`);
  const session = `${cookie[0].name}=${cookie[0].value}`;
  const me = http.get(`${BASE}/api/me`, { headers: { Cookie: session } });
  if (me.status !== 200) fail(`/api/me: ${me.status}`);
  return { session, csrf: me.json('csrf_token'), start: Date.now() };
}

function level(data) {
  const elapsed = (Date.now() - data.start) / 1000;
  const hit = BOUNDS.find((b) => elapsed < b.end);
  return hit ? hit.name : LEVELS[LEVELS.length - 1].name;
}

function params(data, name, tags) {
  return {
    headers: { Cookie: data.session, 'X-CSRF-Token': data.csrf, 'Content-Type': 'application/json' },
    tags: { name, ...tags },
  };
}

export function reads(data) {
  const db = DBS[Math.floor(Math.random() * DBS.length)];
  const tags = { level: level(data), db };
  const pick = Math.random();
  let res;
  if (pick < 0.4) {
    res = http.get(`${BASE}/api/databases/${db}/history?hide_rollback=1`, params(data, 'history', tags));
  } else if (pick < 0.65) {
    res = http.get(`${BASE}/api/databases/${db}/migrations`, params(data, 'migrations', tags));
  } else if (pick < 0.85) {
    res = http.get(`${BASE}/api/databases/${db}/workflow`, params(data, 'board', tags));
  } else {
    res = http.get(`${BASE}/api/approvals`, params(data, 'approvals', tags));
  }
  check(res, { 'read 200': (r) => r.status === 200 });
}

const ms = (a, b) => new Date(b).getTime() - new Date(a).getTime();

export function workflow(data) {
  // VU dibagi rata ke database: VU dengan sisa bagi sama berebut database yang sama.
  const db = DBS[(__VU - 1) % DBS.length];
  const tags = { level: level(data), db };
  const table = `k6_${__VU}_${__ITER}_${Date.now()}`;
  const post = (path, body, name) => http.post(`${BASE}${path}`, JSON.stringify(body), params(data, name, tags));
  const failed = () => { workflowFailed.add(1, tags); sleep(1); };

  const created = post(`/api/databases/${db}/migrations`,
    { name: `k6 ${table}`, up_sql: `CREATE TABLE ${table} (id INT)`, down_sql: `DROP TABLE ${table}` }, 'create');
  if (!check(created, { 'create 201': (r) => r.status === 201 })) return failed();
  const mig = created.json('id');

  const validated = post(`/api/databases/${db}/migrations/${mig}/validate`, {}, 'validate');
  if (!check(validated, { 'validate ok': (r) => r.status === 200 && r.json('ok') === true })) return failed();

  const submitted = post(`/api/databases/${db}/migrations/${mig}/requests`, { action: 'up' }, 'submit');
  if (!check(submitted, { 'submit 201': (r) => r.status === 201 })) return failed();
  const req = submitted.json('id');

  // Respons approve (202 + job_id) bukan tanda migration berhasil; keberhasilan
  // baru dihitung setelah polling melihat status success.
  const approved = post(`/api/migration-requests/${req}/approve`, {}, 'approve');
  if (!check(approved, { 'approve 202': (r) => r.status === 202 })) return failed();

  const start = Date.now();
  while (Date.now() - start < 60000) {
    const res = http.get(`${BASE}/api/migration-requests/${req}`, params(data, 'poll', tags));
    const body = res.status === 200 ? res.json() : null;
    if (body && body.status === 'success') {
      approveToDone.add(Date.now() - start, tags);
      if (body.queued_at && body.started_at) queueWait.add(Math.max(0, ms(body.queued_at, body.started_at)), tags);
      if (body.started_at && body.finished_at) jobDuration.add(Math.max(0, ms(body.started_at, body.finished_at)), tags);
      jobsDone.add(1, tags);
      return;
    }
    if (body && ['failed', 'partial', 'rejected', 'cancelled', 'canceled'].includes(body.status)) return failed();
    sleep(0.25);
  }
  pollTimeout.add(1, tags);
  failed();
}
