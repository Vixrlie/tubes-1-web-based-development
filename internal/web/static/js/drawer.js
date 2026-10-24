(function () {
    'use strict';

    var app = document.querySelector('.app');
    if (!app) return;

    var openButton = app.querySelector('[data-drawer-open]');
    var closeButtons = app.querySelectorAll('[data-drawer-close]');
    var sidebar = app.querySelector('.app-sidebar');
    var backdrop = app.querySelector('.drawer-backdrop');

    function isOpen() {
        return app.getAttribute('data-drawer') === 'open';
    }

    function setOpen(open) {
        app.setAttribute('data-drawer', open ? 'open' : 'closed');
        if (openButton) {
            openButton.setAttribute('aria-expanded', String(open));
        }
        document.body.style.overflow = open ? 'hidden' : '';

        if (open && sidebar) {
            var first = sidebar.querySelector('a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled])');
            if (first) {
                first.focus();
            }
        } else if (!open && openButton) {
            openButton.focus();
        }
    }

    openButton?.addEventListener('click', function () {
        setOpen(!isOpen());
    });

    app.querySelectorAll('[data-nav-toggle]').forEach(function (el) {
        el.addEventListener('click', function (e) {
            e.preventDefault();
            var expanded = el.getAttribute('data-expanded') === 'true';
            el.setAttribute('data-expanded', String(!expanded));
        });
    });

    closeButtons.forEach(function (element) {
        element.addEventListener('click', function () {
            setOpen(false);
        });
    });

    document.addEventListener('keydown', function (event) {
        if (!isOpen() || !sidebar) return;

        if (event.key === 'Escape') {
            setOpen(false);
            return;
        }

        if (event.key !== 'Tab') return;

        var focusable = Array.prototype.slice.call(sidebar.querySelectorAll('a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled])'));
        if (focusable.length === 0) return;

        var first = focusable[0];
        var last = focusable[focusable.length - 1];

        if (event.shiftKey && document.activeElement === first) {
            event.preventDefault();
            last.focus();
        } else if (!event.shiftKey && document.activeElement === last) {
            event.preventDefault();
            first.focus();
        }
    });

    window.addEventListener('resize', function () {
        if (window.innerWidth >= 768 && isOpen()) {
            setOpen(false);
        }
    });

    if (window.innerWidth >= 768) {
        setOpen(false);
    }
}());
