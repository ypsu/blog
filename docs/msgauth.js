function reportError(e) {
  herror.innerText = 'error: ' + e
  herror.hidden = false
}

async function main() {
  window.onerror = (msg, src, line) => reportError(`${src}:${line} ${msg}`)
  window.onunhandledrejection = e => reportError(e.reason)

  let id1 = 0, id2 = 0, id3 = 0
  while (id1 < 100) id1 = Math.trunc(Math.random() * 1000)
  while (id2 < 100) id2 = Math.trunc(Math.random() * 1000)
  while (id3 < 100) id3 = Math.trunc(Math.random() * 1000)
  let id = `${id1}-${id2}-${id3}`
  let email = `msgauth@${document.domain}`

  hcode.innerText = id
  hdomain.innerText = document.domain
  hlink.href = `mailto:${email}?subject=${id}`
  new QRCode(hqrcode, `mailto:${email}?subject=${id}`)
  hloading.hidden = true
  hdemo.hidden = false

  //let response = await fetch(`${location.origin}/msgauthwait?id=${id}`)
  let response = await fetch(`https://notech.ie/msgauthwait?id=${id}`)
  if (response.status != 200) {
    reportError(`unexpected status ${response.status}`)
    return
  }
  let user = await response.text()
  hgreeting.innerText = `email auth successful! hello ${user}!`
  hgreeting.hidden = false
  hdemo.hidden = true
}

main()
