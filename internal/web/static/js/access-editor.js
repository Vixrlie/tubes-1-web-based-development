// Editor akses database (templates/components/access-editor.html).
// Mengumpulkan perubahan di sisi klien (ubah level, cabut / batal cabut, tambah database)
// lalu menampilkan action bar "N perubahan akses belum disimpan". Data dikirim sekali
// lewat form #access-form saat "Simpan Perubahan" ditekan; "Batal" hanya link kembali.
(function () {
    "use strict";

    function init(root) {
        var list = root.querySelector("[data-list]");
        var tpl = root.querySelector("template[data-new-row]");
        var addBtn = root.querySelector("[data-add]");
        var bar = document.getElementById(root.getAttribute("data-bar"));
        var barText = bar && bar.querySelector(".action-bar__text");
        var format = root.getAttribute("data-message") || "{n}";
        var seq = 0;
        var maxNew = tpl
            ? Array.prototype.filter.call(
                tpl.content.querySelectorAll("select[name='add_db'] option"),
                function (o) { return o.value !== ""; }
            ).length
            : 0;

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

        // database yang sudah dipilih di baris baru lain tidak boleh dipilih lagi
        function syncChoices() {
            var chosen = [];
            newRows().forEach(function (row) {
                var v = row.querySelector("select[name='add_db']").value;
                if (v) chosen.push(v);
            });
            newRows().forEach(function (row) {
                var sel = row.querySelector("select[name='add_db']");
                Array.prototype.forEach.call(sel.options, function (o) {
                    o.disabled = o.value === "" || (o.value !== sel.value && chosen.indexOf(o.value) !== -1);
                });
            });
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

        function addRow() {
            if (!tpl || (addBtn && addBtn.disabled)) return;
            var row = tpl.content.firstElementChild.cloneNode(true);
            var n = ++seq;
            // id unik per baris supaya <label for> tetap cocok
            row.querySelectorAll("select").forEach(function (sel) {
                var label = sel.closest(".input").querySelector("label");
                sel.id = sel.id + "-" + n;
                if (label) label.setAttribute("for", sel.id);
            });
            list.appendChild(row);
            syncChoices();
            update();
            row.querySelector("select").focus();
        }

        root.addEventListener("click", function (e) {
            var t = e.target.closest("button");
            if (!t || !root.contains(t)) return;
            var row = t.closest("[data-row]");
            if (t.hasAttribute("data-add")) {
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

        root.addEventListener("change", function () {
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
