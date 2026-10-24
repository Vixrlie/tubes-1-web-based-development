(function () {
    'use strict';

    var forms = document.querySelectorAll('[data-confirm-form]');
    Array.prototype.forEach.call(forms, function (form) {
        var expected = form.getAttribute('data-confirm-value') || '';
        var nameInput = form.querySelector('[data-confirm-name]');
        var checkInput = form.querySelector('[data-confirm-check]');
        var submit = form.querySelector('[data-confirm-submit]');

        if (!nameInput || !checkInput || !submit) {
            return;
        }

        function refresh() {
            var ready = nameInput.value.trim() === expected && checkInput.checked;
            submit.disabled = !ready;
            submit.setAttribute('aria-disabled', ready ? 'false' : 'true');
        }

        nameInput.addEventListener('input', refresh);
        checkInput.addEventListener('change', refresh);
        refresh();
    });
}());
