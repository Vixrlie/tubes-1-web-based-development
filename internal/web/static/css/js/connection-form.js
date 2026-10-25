// Form Edit Koneksi (templates/admin/connection_form.html).
// 1. Action bar (#connection-bar) tampil begitu ada isian yang berbeda dari nilai awal,
//    sembunyi lagi kalau dikembalikan.
// 2. Hasil tes koneksi yang gagal membawa data-invalid-field="<name>": input itu disorot
//    merah (juga setelah htmx mengganti hasil tes). Mengetik di input itu menghapus sorotan.
(function () {
    "use strict";

    function init(form) {
        var bar = document.getElementById(form.getAttribute("data-bar"));
        var fields = form.querySelectorAll("input:not([type=hidden])");
        var initial = {};
        Array.prototype.forEach.call(fields, function (f) { initial[f.name] = f.value; });

        function dirty() {
            return Array.prototype.some.call(fields, function (f) { return f.value !== initial[f.name]; });
        }
        function syncBar() {
            if (bar) bar.hidden = !dirty();
        }

        function boxOf(input) { return input && input.closest(".input"); }

        function clearTestErrors() {
            form.querySelectorAll(".input[data-test-error]").forEach(function (box) {
                box.classList.remove("is-error");
                box.removeAttribute("data-test-error");
                var inp = box.querySelector("input");
                if (inp) inp.removeAttribute("aria-invalid");
            });
        }

        function applyResult() {
            clearTestErrors();
            var res = form.querySelector("[data-invalid-field]");
            if (!res) return;
            var inp = form.elements[res.getAttribute("data-invalid-field")];
            var box = boxOf(inp);
            if (!box) return;
            box.classList.add("is-error");
            box.setAttribute("data-test-error", "");
            inp.setAttribute("aria-invalid", "true");
        }

        form.addEventListener("input", function (e) {
            var box = boxOf(e.target);
            if (box && box.hasAttribute("data-test-error")) {
                box.classList.remove("is-error");
                box.removeAttribute("data-test-error");
                e.target.removeAttribute("aria-invalid");
            }
            syncBar();
        });

        // htmx mengganti tombol + hasil tes -> sorot ulang field yang bermasalah
        document.body.addEventListener("htmx:afterSettle", applyResult);

        // tombol back (bfcache) bisa memulihkan isian -> hitung ulang
        window.addEventListener("pageshow", syncBar);

        applyResult();
        syncBar();
    }

    document.querySelectorAll("form[data-bar]").forEach(init);
})();
