(function () {
    'use strict';

    var hosts = document.querySelectorAll('[data-sql-editor]');
    Array.prototype.forEach.call(hosts, function (host) {
        if (typeof window.createSqlEditor !== 'function') {
            return;
        }

        var form = host.closest('form');
        var source = form ? form.querySelector('[data-sql-source]') : null;
        if (!source) {
            return;
        }

        var editor = window.createSqlEditor(host, {
            doc: source.value,
            onChange: function (value) {
                source.value = value;
            }
        });

        var content = editor.view.contentDOM;
        content.setAttribute('role', 'textbox');
        content.setAttribute('aria-multiline', 'true');
        content.setAttribute('aria-label', host.getAttribute('data-label') || 'SQL migration');

        var describedBy = host.getAttribute('data-describedby');
        if (describedBy) {
            content.setAttribute('aria-describedby', describedBy);
        }

        source.hidden = true;
        source.setAttribute('aria-hidden', 'true');

        form.addEventListener('submit', function () {
            source.value = editor.getValue();
        });
    });
}());
