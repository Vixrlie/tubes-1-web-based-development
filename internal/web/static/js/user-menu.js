// Kartu akun di header (partials/header.html): avatar membuka / menutup kartu.
// Menutup lewat: klik avatar lagi, klik di luar kartu, tombol Esc (fokus kembali ke avatar),
// atau fokus keyboard (Tab) keluar dari kartu.
(function () {
    "use strict";

    function init(root) {
        var btn = root.querySelector(".user-menu__trigger");
        var panel = root.querySelector(".user-menu__panel");
        if (!btn || !panel) return;

        function setOpen(open, refocus) {
            panel.hidden = !open;
            btn.setAttribute("aria-expanded", open ? "true" : "false");
            if (!open && refocus) btn.focus();
        }

        btn.addEventListener("click", function () {
            setOpen(panel.hidden);
        });

        document.addEventListener("click", function (e) {
            if (!panel.hidden && !root.contains(e.target)) setOpen(false);
        });

        document.addEventListener("keydown", function (e) {
            if (e.key === "Escape" && !panel.hidden) setOpen(false, true);
        });

        root.addEventListener("focusout", function (e) {
            if (!panel.hidden && e.relatedTarget && !root.contains(e.relatedTarget)) setOpen(false);
        });

        // kembali lewat tombol back (bfcache) -> kartu selalu mulai tertutup
        window.addEventListener("pageshow", function () {
            setOpen(false);
        });
    }

    document.querySelectorAll("[data-user-menu]").forEach(init);
})();