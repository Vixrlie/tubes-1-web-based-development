// Pop up "Tambah Anggota" (templates/components/add-members-modal.html).
// - kotak cari menyaring daftar berdasarkan nama / username (huruf besar-kecil tidak berpengaruh)
// - tombol utama berubah jadi "Tambahkan N anggota" dan mati kalau belum ada yang dicentang
// Pengiriman data tetap form POST biasa (field user_id berulang), bukan fetch.
// Perlu static/js/modal.js untuk buka / tutup / reset modal.
(function () {
    "use strict";

    function init(dlg) {
        var form = dlg.querySelector("form");
        var search = dlg.querySelector("[data-member-search]");
        var items = dlg.querySelectorAll("[data-member-item]");
        var noMatch = dlg.querySelector("[data-member-nomatch]");
        var submit = dlg.querySelector("[data-member-submit]");
        if (!form || !submit) return;

        var label = submit.querySelector(".btn__label");
        var zero = dlg.getAttribute("data-label-zero") || "Tambahkan";
        var format = dlg.getAttribute("data-label-format") || "Tambahkan {n} anggota";

        function selected() {
            return form.querySelectorAll("input[name='user_id']:checked:not(:disabled)").length;
        }

        function update() {
            var n = selected();
            if (label) label.textContent = n > 0 ? format.replace("{n}", n) : zero;
            submit.disabled = n === 0;
            submit.classList.toggle("is-disabled", n === 0);
        }

        // baris yang tersaring hanya disembunyikan (atribut hidden), centangnya tetap ikut terkirim
        function filter() {
            var q = search ? search.value.trim().toLowerCase() : "";
            var shown = 0;
            items.forEach(function (li) {
                var ok = q === "" || (li.getAttribute("data-search") || "").indexOf(q) !== -1;
                li.hidden = !ok;
                if (ok) shown++;
            });
            if (noMatch) noMatch.hidden = items.length === 0 || shown > 0;
        }

        if (search) search.addEventListener("input", filter);
        form.addEventListener("change", update);
        form.addEventListener("submit", function (e) {
            if (selected() === 0) e.preventDefault();
        });

        // modal.js me-reset form saat ditutup -> hitung ulang daftar dan tombol
        dlg.addEventListener("modal:reset", function () {
            filter();
            update();
        });

        // kembali lewat tombol back (bfcache) -> sesuaikan dengan state form
        window.addEventListener("pageshow", function () {
            filter();
            update();
        });

        filter();
        update();
    }

    document.querySelectorAll("[data-add-members]").forEach(init);
})();