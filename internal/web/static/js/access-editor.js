// Editor akses database (templates/components/access-editor.html).
// Mengumpulkan perubahan di sisi klien (ubah level, cabut / batal cabut, tambah database)
// lalu menampilkan action bar "N perubahan akses belum disimpan". Data dikirim sekali
// lewat form #access-form saat "Simpan Perubahan" ditekan; "Batal" hanya link kembali.
//
// "Tambah Database" membuka pop up (pilih database + level akses). Menekan "Tambahkan" di pop up
// belum menyimpan apa pun: baris baru hanya ditambahkan ke daftar dan dihitung sebagai satu
// perubahan. Perlu static/js/modal.js untuk tombol tutup / Batal / X di pop up.
(function () {
    "use strict";

    function init(root) {
        var list = root.querySelector("[data-list]");
        var tpl = root.querySelector("template[data-new-row]");
        var addBtn = root.querySelector("[data-add]");
        var bar = document.getElementById(root.getAttribute("data-bar"));
        var barText = bar && bar.querySelector(".action-bar__text");
        var format = root.getAttribute("data-message") || "{n}";

        var modal = root.querySelector("[data-add-db-modal]");
        var modalDb = modal && modal.querySelector("[data-modal-db]");
        var confirmBtn = modal && modal.querySelector("[data-modal-confirm]");
        var seq = 0;

        // pilihan yang memang boleh ditambahkan (tidak dinonaktifkan server) = batas baris baru
        var maxNew = 0;
        if (modalDb) {
            Array.prototype.forEach.call(modalDb.options, function (o) {
                o.setAttribute("data-server-disabled", o.disabled || o.value === "" ? "1" : "");
                if (!o.disabled && o.value !== "") maxNew++;
            });
        }

        function rows() { return list.querySelectorAll("[data-row]"); }
        function newRows() { return list.querySelectorAll("[data-new]"); }

        // jumlah perubahan: level berubah, database dicabut, atau baris baru
        function count() {
            var n = 0;
            rows().forEach(function (row) {
                var sel = row.querySelector("select");
                if (row.hasAttribute("data-new") || row.getAttribute("data-state") === "removed") {
                    n++;
                } else if (sel && sel.value !== row.getAttribute("data-initial")) {
                    n++;
                }
            });
            return n;
        }

        // database yang sudah ditambahkan sebagai baris baru tidak muncul lagi di pop up
        function syncChoices() {
            if (!modalDb) return;
            var chosen = [];
            newRows().forEach(function (row) {
                var v = row.querySelector("[data-new-db]").value;
                if (v) chosen.push(v);
            });
            var firstFree = null;
            Array.prototype.forEach.call(modalDb.options, function (o) {
                o.disabled = !!o.getAttribute("data-server-disabled") || chosen.indexOf(o.value) !== -1;
                if (!o.disabled && !firstFree) firstFree = o;
            });
            var cur = modalDb.options[modalDb.selectedIndex];
            if (!cur || cur.disabled) modalDb.value = firstFree ? firstFree.value : "";
            if (confirmBtn) {
                confirmBtn.disabled = !firstFree;
                confirmBtn.classList.toggle("is-disabled", !firstFree);
            }
        }

        function update() {
            var n = count();
            if (bar) {
                bar.hidden = n === 0;
                if (barText) barText.textContent = format.replace("{n}", n);
            }
            if (addBtn) {
                var full = newRows().length >= maxNew;
                addBtn.disabled = full;
                addBtn.classList.toggle("is-disabled", full);
            }
        }

        function setRemoved(row, removed) {
            var sel = row.querySelector("select");
            var field = row.querySelector("[data-remove-field]");
            if (removed) row.setAttribute("data-state", "removed");
            else row.removeAttribute("data-state");
            sel.disabled = removed;
            sel.closest(".dropdown").classList.toggle("is-disabled", removed);
            field.disabled = !removed; // hanya dikirim kalau dicabut
        }

        // Pop up Tambah Database
        function openModal() {
            if (!modal || (addBtn && addBtn.disabled)) return;
            syncChoices();
            var view = modal.querySelector("input[type='radio'][value='view']");
            if (view) view.checked = true; // selalu mulai dari level paling terbatas
            if (typeof modal.showModal === "function") modal.showModal();
            else modal.setAttribute("open", "");
        }

        function closeModal() {
            if (!modal || !modal.open) return;
            if (typeof modal.close === "function") modal.close();
            else modal.removeAttribute("open");
        }

        function addRow() {
            if (!tpl || !modalDb) return;
            var opt = modalDb.options[modalDb.selectedIndex];
            if (!opt || opt.disabled || opt.value === "") return;
            var levelInput = modal.querySelector("input[type='radio']:checked");
            var level = levelInput ? levelInput.value : "view";

            var row = tpl.content.firstElementChild.cloneNode(true);
            var n = ++seq;
            row.querySelector("[data-new-name]").textContent = opt.textContent;
            row.querySelector("[data-new-db]").value = opt.value;
            // id unik per baris supaya <label for> tetap cocok
            row.querySelectorAll("select").forEach(function (sel) {
                var label = sel.closest(".input").querySelector("label");
                sel.id = sel.id + "-" + n;
                if (label) label.setAttribute("for", sel.id);
                sel.value = level;
            });
            list.appendChild(row);
            syncChoices();
            update();
            closeModal();
            var sel = row.querySelector("select");
            if (sel) sel.focus();
        }

        root.addEventListener("click", function (e) {
            var t = e.target.closest("button");
            if (!t || !root.contains(t)) return;
            var row = t.closest("[data-row]");
            if (t.hasAttribute("data-add")) {
                openModal();
            } else if (t.hasAttribute("data-modal-confirm")) {
                addRow();
            } else if (row && t.hasAttribute("data-remove")) {
                if (row.hasAttribute("data-new")) row.remove(); // baris baru: buang saja
                else setRemoved(row, true);
                syncChoices();
                update();
            } else if (row && t.hasAttribute("data-restore")) {
                setRemoved(row, false);
                update();
            }
        });

        root.addEventListener("change", function (e) {
            // perubahan di dalam pop up belum menjadi perubahan akses
            if (modal && modal.contains(e.target)) return;
            syncChoices();
            update();
        });

        // kembali lewat tombol back (bfcache) -> hitung ulang dari state form
        window.addEventListener("pageshow", update);
        syncChoices();
        update();
    }

    document.querySelectorAll("[data-access-editor]").forEach(init);
})();