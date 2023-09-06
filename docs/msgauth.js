function reportError(e) {
  herror.innerText = 'error: ' + e
  herror.hidden = false
  hdemo.hidden = true
}

function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms))
}

async function main() {
  if (document.domain != 'iio.ie') {
    hloading.innerText = 'this demo only works on the primary site.'
    return
  }

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

  let user, err, attempt
  for (attempt = 0; attempt < 5; attempt++) {
    try {
      let response = await fetch(`https://iio.ie/msgauthwait?id=${id}`)
      if (response.status == 200) {
        user = await response.text()
        break
      }
      err = `unexpected status ${response.status}`
    } catch (e) {
      err = e
    }
    await sleep(1000)
  }
  if (attempt == 5) {
    reportError(err)
    return
  }
  hgreeting.innerText = `email auth successful! hello ${user}!`
  hgreeting.hidden = false
  hdemo.hidden = true
}

main()
