// Stress test workflow migration Pathgres.
//
// Skenario:
//   reads   : riwayat (hide rollback), daftar migration, inbox approval.
//   workflow: create -> validate -> submit -> approve -> tunggu worker selesai.
//
// Variabel lingkungan:
//   BASE_URL  (default http://localhost:8080)
//   USERNAME, PASSWORD  user dengan akses View + Edit dan leader satu-satunya
//                       group yang memiliki akses ke DB_ID
//   DB_ID     id koneksi database target
//   READ_RATE, WORKFLOW_VUS, DURATION  intensitas beban
import http from 'k6/http';
import { check, fail, sleep } from 'k6';
import { Counter, Trend } from 'k6/metrics';

const BASE = __ENV.BASE_URL || 'http://localhost:8080';
const DB = __ENV.DB_ID || fail('DB_ID wajib diisi');
const DURATION = __ENV.DURATION || '1m';

const queueToDone = new Trend('workflow_queue_to_done_ms', true);
const workflowFailed = new Counter('workflow_failed');

export const options = {
  scenarios: {
    reads: {
      executor: 'constant-arrival-rate',
      exec: 'reads',
      rate: Number(__ENV.READ_RATE || 50),
      timeUnit: '1s',
      duration: DURATION,
      preAllocatedVUs: 20,
      maxVUs: 100,
    },
    workflow: {
      executor: 'constant-vus',
      exec: 'workflow',
      vus: Number(__ENV.WORKFLOW_VUS || 5),
      duration: DURATION,
    },
  },
  thresholds: {
    'http_req_failed{scenario:reads}': ['rate<0.01'],
    'http_req_duration{scenario:reads}': ['p(95)<300'],
    'http_req_duration{name:approve}': ['p(95)<500'],
    workflow_failed: ['count==0'],
  },
};

// setup login sekali; cookie sesi dan token CSRF dibagikan ke semua VU.
export function setup() {
  const res = http.post(`${BASE}/login`, { username: __ENV.USERNAME, password: __ENV.PASSWORD }, { redirects: 0 });
  const cookie = res.cookies.pathgres_session;
  if (!cookie || cookie.length === 0) fail(`login gagal: status ${res.status}`);
  const session = `${cookie[0].name}=${cookie[0].value}`;
  const me = http.get(`${BASE}/api/me`, { headers: { Cookie: session } });
  if (me.status !== 200) fail(`/api/me: ${me.status}`);
  return { session, csrf: me.json('csrf_token') };
}

function headers(data, name) {
  return {
    headers: { Cookie: data.session, 'X-CSRF-Token': data.csrf, 'Content-Type': 'application/json' },
    tags: { name },
  };
}

export function reads(data) {
  const pick = Math.random();
  let res;
  if (pick < 0.5) {
    res = http.get(`${BASE}/api/databases/${DB}/history?hide_rollback=1`, headers(data, 'history'));
  } else if (pick < 0.8) {
    res = http.get(`${BASE}/api/databases/${DB}/migrations`, headers(data, 'migrations'));
  } else {
    res = http.get(`${BASE}/api/approvals`, headers(data, 'approvals'));
  }
  check(res, { 'read 200': (r) => r.status === 200 });
}

export function workflow(data) {
  const table = `k6_${__VU}_${__ITER}_${Date.now()}`;
  const post = (path, body, name) => http.post(`${BASE}${path}`, JSON.stringify(body), headers(data, name));

  const created = post(`/api/databases/${DB}/migrations`,
    { name: `k6 ${table}`, up_sql: `CREATE TABLE ${table} (id INT)`, down_sql: `DROP TABLE ${table}` }, 'create');
  if (!check(created, { 'create 201': (r) => r.status === 201 })) { workflowFailed.add(1); return; }
  const mig = created.json('id');

  const validated = post(`/api/databases/${DB}/migrations/${mig}/validate`, {}, 'validate');
  if (!check(validated, { 'validate ok': (r) => r.status === 200 && r.json('ok') === true })) { workflowFailed.add(1); return; }

  const submitted = post(`/api/databases/${DB}/migrations/${mig}/requests`, { action: 'up' }, 'submit');
  if (!check(submitted, { 'submit 201': (r) => r.status === 201 })) { workflowFailed.add(1); return; }
  const req = submitted.json('id');

  const approved = post(`/api/migration-requests/${req}/approve`, {}, 'approve');
  if (!check(approved, { 'approve 202': (r) => r.status === 202 })) { workflowFailed.add(1); return; }

  // Approve tidak menunggu SQL; ukur waktu sampai worker menyelesaikan job.
  const start = Date.now();
  for (let i = 0; i < 60; i++) {
    const res = http.get(`${BASE}/api/migration-requests/${req}`, headers(data, 'poll'));
    const status = res.json('status');
    if (status === 'success') { queueToDone.add(Date.now() - start); return; }
    if (status === 'failed' || status === 'cancelled') { workflowFailed.add(1); return; }
    sleep(0.25);
  }
  workflowFailed.add(1);
}
