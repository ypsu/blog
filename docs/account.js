import { iio, strings } from "./iio.js";
async function login(event) {
    event.preventDefault();
    eLoginStatus.textContent = "Logging in...";
    let [r, err] = await iio.Fetch("/userapi?action=login", {
        method: "POST",
        headers: { "Content-Type": "application/x-www-form-urlencoded" },
        body: `username=${eLoginUsername.value}&password=${encodeURIComponent(eLoginPassword.value)}`,
    });
    if (err.includes("userapi.LoginUsernameNotFound")) {
        eLoginStatus.textContent = "Error: user not found";
        return;
    }
    if (err.includes("userapi.LoginUsernameOrPasswordMissing")) {
        eLoginStatus.textContent = "Error: incomplete form";
        return;
    }
    if (err.includes("userapi.InvalidUsernameCharacter") || err.includes("UsernameTooLong")) {
        eLoginStatus.textContent = "Error: invalid username";
        return;
    }
    if (err.includes("userapi.BadPassword")) {
        eLoginStatus.textContent = "Error: wrong password";
        return;
    }
    if (err != "") {
        eLoginStatus.textContent = "Error: " + err;
        return;
    }
    eLoginStatus.textContent = "";
    iio.User = eLoginUsername.value;
    let [pubnote, privnote] = strings.Cut(r, "\n");
    eUpdatePubnote.value = pubnote;
    eUpdatePrivnote.value = privnote;
    render();
}
async function register(event) {
    event.preventDefault();
    eRegisterStatus.textContent = "Registering...";
    let [_, err] = await iio.Fetch("/userapi?action=register", {
        method: "POST",
        headers: { "Content-Type": "application/x-www-form-urlencoded" },
        body: new URLSearchParams({
            username: eRegisterUsername.value,
            password: eRegisterPassword.value,
            pubnote: eRegisterPubnote.value,
            privnote: eRegisterPrivnote.value,
        }).toString(),
    });
    if (err != "") {
        eRegisterStatus.textContent = "Error: " + err;
        return;
    }
    eRegisterStatus.textContent = "";
    eUpdatePubnote.value = eRegisterPubnote.value;
    eUpdatePrivnote.value = eRegisterPrivnote.value;
    iio.User = eRegisterUsername.value;
    render();
}
async function update(event) {
    event.preventDefault();
    eUpdateStatus.textContent = "Updating...";
    let [_, err] = await iio.Fetch("/userapi?action=update", {
        method: "POST",
        headers: { "Content-Type": "application/x-www-form-urlencoded" },
        body: new URLSearchParams({
            pubnote: eUpdatePubnote.value,
            privnote: eUpdatePrivnote.value,
        }).toString(),
    });
    if (err != "") {
        eUpdateStatus.textContent = "Error: " + err;
        return;
    }
    eUpdateStatus.textContent = "";
}
async function updatePassword(event) {
    event.preventDefault();
    eUpdatePasswordStatus.textContent = "Updating...";
    let [_, err] = await iio.Fetch("/userapi?action=update", {
        method: "POST",
        headers: { "Content-Type": "application/x-www-form-urlencoded" },
        body: new URLSearchParams({
            oldpassword: eUpdateOldPassword.value,
            newpassword: eUpdateNewPassword.value,
        }).toString(),
    });
    if (err != "") {
        eUpdatePasswordStatus.textContent = "Error: " + err;
        return;
    }
    eUpdatePasswordStatus.textContent = "Password changed.";
    eUpdateOldPassword.value = "";
    eUpdateNewPassword.value = "";
}
async function logout(event) {
    event.preventDefault();
    eLogoutStatus.textContent = "Logging out...";
    let [_, err] = await iio.Fetch("/userapi?action=logout", { method: "POST" });
    if (err != "") {
        eLogoutStatus.textContent = "Error: " + err;
        return;
    }
    eLogoutStatus.textContent = "";
    iio.User = "";
    render();
}
function render() {
    eJSWarning.hidden = true;
    eLogin.hidden = true;
    eManage.hidden = true;
    if (iio.User == "") {
        eLogin.hidden = false;
        eCurrentLogin.textContent = "You are currently not logged in.";
    }
    else if (iio.User.endsWith("-guest")) {
        eLogin.hidden = false;
        eCurrentLogin.textContent = `You are currently logged in as ${iio.User}. Log in to / create a full account below.`;
    }
    else {
        eManage.hidden = false;
        eCurrentLogin.textContent = `You are currently logged in as ${iio.User}.`;
    }
    let disabled = !eDangerZoneKnob.checked;
    for (let e of document.querySelectorAll("#eDangerZone input"))
        e.disabled = disabled;
}
async function init() {
    eLoginButton.onclick = login;
    eRegisterButton.onclick = register;
    eUpdateButton.onclick = update;
    eUpdatePasswordButton.onclick = updatePassword;
    eLogoutButton.onclick = logout;
    eDangerZoneKnob.onchange = render;
    if (iio.User == "")
        return;
    eUpdateStatus.textContent = "Loading user data...";
    let [r, err] = await iio.Fetch("/userapi?action=userdata", { method: "POST" });
    if (err != "") {
        eUpdateStatus.textContent = "Error: " + err;
        return;
    }
    let [pubnote, privnote] = strings.Cut(r, "\n");
    eUpdatePubnote.value = pubnote;
    eUpdatePrivnote.value = privnote;
    eUpdateStatus.textContent = "";
    render();
}
iio.Init();
render();
init();
