(function () {
    'use strict';

    if (typeof window.htmx === 'undefined') return;

    document.body.addEventListener('htmx:configRequest', function (event) {
        const token = document.querySelector('meta[name="csrf-token"]')?.getAttribute('content')
            || document.querySelector('input[name="csrf_token"]')?.value;
        if (token) {
            event.detail.headers['X-CSRF-Token'] = token;
        }
    });

    document.body.addEventListener('htmx:confirm', function (event) {
        const prompt = event.detail.elt?.getAttribute('data-confirm');
        if (!prompt) return;

        event.preventDefault();
        if (window.confirm(prompt)) {
            event.detail.issueRequest(true);
        }
    });

    document.body.addEventListener('htmx:afterRequest', function (event) {
        const elt = event.detail.elt;
        const refresh = elt?.getAttribute('data-refresh-on-success');
        if (refresh && event.detail.successful) {
            window.location.reload();
        }
    });
}());
