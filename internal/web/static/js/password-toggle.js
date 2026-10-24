// Tombol tampil/sembunyikan password.
// Dipakai komponen input dengan PasswordToggle=true: tombol [data-password-toggle]
// membalik type input di sebelahnya (password <-> text) dan aria-pressed.
document.addEventListener("click", function (e) {
    var btn = e.target.closest("[data-password-toggle]");
    if (!btn) return;
    var box = btn.closest(".input__box");
    var input = box && box.querySelector("input");
    if (!input) return;
    var show = input.type === "password";
    input.type = show ? "text" : "password";
    btn.setAttribute("aria-pressed", show ? "true" : "false");
    btn.setAttribute("aria-label", show ? "Sembunyikan password" : "Tampilkan password");
});