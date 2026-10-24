// Nomor baris untuk panel SQL statis (halaman detail & pratinjau migration).
// Dipicu oleh atribut data-sql-lines pada <pre>; konten SQL dimasukkan lewat
// textContent sehingga tidak pernah diinterpretasi sebagai HTML.
(function () {
  function numberLines(pre) {
    var code = pre.querySelector('code');
    if (!code || code.querySelector('.sql-line')) return;
    var text = code.textContent.replace(/\n$/, '');
    var lines = text.split('\n');
    code.textContent = '';
    lines.forEach(function (line, i) {
      var row = document.createElement('span');
      row.className = 'sql-line';

      var no = document.createElement('span');
      no.className = 'sql-line-no';
      no.setAttribute('aria-hidden', 'true');
      no.textContent = String(i + 1);

      var body = document.createElement('span');
      body.className = 'sql-line-text';
      body.textContent = line;

      row.appendChild(no);
      row.appendChild(body);
      code.appendChild(row);
    });
  }

  function run() {
    document.querySelectorAll('[data-sql-lines]').forEach(numberLines);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', run);
  } else {
    run();
  }
})();
