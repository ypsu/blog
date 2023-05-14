function main() {
  let id = 0
  while (id < 1e8) id = Math.trunc(Math.random() * 1e9)
  let email = `msgauth@${document.domain}`
  hcode.innerHTML = `send ${id} to ${email}. <a href="mailto:${email}?subject=${id}">click here</a> or scan this:`
  new QRCode(hqrcode, `mailto:${email}?subject=${id}`)
}

main()
