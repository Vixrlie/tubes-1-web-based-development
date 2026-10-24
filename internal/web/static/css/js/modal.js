// Buka / tutup pop up (templates/components/modal.html), berbasis <dialog> bawaan browser.
//
//   data-modal-open="<id>"  tombol / link pembuka. Kalau dialog-nya tidak ada di halaman,
//                           klik dibiarkan jalan (mis. link href cadangan tetap diikuti).
//   data-modal-close        tombol penutup di dalam modal.
//   data-modal-reset        pada <form> di dalam modal: form di-reset setiap modal tertutup,
//                           lalu event "modal:reset" dikirim ke <dialog> supaya JS komponen
//                           bisa menghitung ulang tampilannya.
//
// Esc dan fokus-terkunci ditangani browser. Klik latar gelap menutup modal.
(function () {
    "use strict";

    function openDialog(dlg) {
        if (!dlg || dlg.open) return;
        if (typeof dlg.showModal === "function") dlg.showModal();
        else dlg.setAttribute("open", "");
    }

    function closeDialog(dlg) {
        if (!dlg || !dlg.open) return;
        if (typeof dlg.close === "function") dlg.close();
        else dlg.removeAttribute("open");
    }

    // klik latar dihitung hanya kalau tombol mouse ditekan DAN dilepas di latar, supaya
    // menyeret teks dari dalam panel sampai keluar tidak menutup modal.
    var downOnBackdrop = false;

    document.addEventListener("mousedown", function (e) {
        downOnBackdrop = e.target.classList && e.target.classList.contains("modal");
    });

    document.addEventListener("click", function (e) {
        var opener = e.target.closest("[data-modal-open]");
        if (opener) {
            var target = document.getElementById(opener.getAttribute("data-modal-open"));
            if (target) {
                e.preventDefault();
                openDialog(target);
            }
            return;
        }

        var closer = e.target.closest("[data-modal-close]");
        if (closer) {
            closeDialog(closer.closest("dialog"));
            return;
        }

        if (downOnBackdrop && e.target.classList && e.target.classList.contains("modal")) {
            closeDialog(e.target);
        }
        downOnBackdrop = false;
    });

    // event "close" tidak bubbling, jadi didengarkan di fase capture
    document.addEventListener("close", function (e) {
        var dlg = e.target;
        if (!dlg || dlg.tagName !== "DIALOG") return;
        dlg.querySelectorAll("form[data-modal-reset]").forEach(function (f) { f.reset(); });
        dlg.dispatchEvent(new CustomEvent("modal:reset"));
    }, true);
})();