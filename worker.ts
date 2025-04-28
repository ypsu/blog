// source code for the cloudflare worker.
// run `wrangler dev -r` do develop.
// run `wrangler deploy` to deploy.
// run `wrangler tail` for the prod logs.
// run `rm -rf ~/.npm` to get a new version of wrangler.

export default {
  email: handleEmail,
  fetch: handleFetch,
}

function response(code: number, message: string) {
  return new Response(message, {
    status: code,
  })
}

async function handleFetch(request: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
  let method = request.method
  let path = new URL(request.url).pathname
  let params = new URL(request.url).searchParams
  if (env.devenv == 0 && request.headers.get("apikey") != env.apikey) {
    return response(403, "unathorized")
  }

  let value, list, r
  switch (true) {
    case path == "/api/alogdb" && method == "GET": {
      let results = await env.DB.prepare("select ts, name, log from alogdb order by ts asc").raw()
      let r = ""
      for (let e of results) r += `${e[0]} ${e[1]} ${e[2]}\0\n`
      return response(200, r)
    }

    case path == "/api/alogdb" && method == "POST": {
      let name = params.get("name")
      if (name == null) return response(400, "worker.MissingADBName")
      let ts = params.get("ts")
      if (ts == null) return response(400, "worker.MissingADBTS")
      let body = await request.text()
      let texts = body.split("\x00")
      let sql = "insert into alogdb (ts, name, log) values " + Array(texts.length).fill("(?, ?, ?)").join(", ")
      let items = []
      for (let text of texts) items.push(ts++, name, text)
      await env.DB.prepare(sql)
        .bind(...items)
        .run()
      return response(200, "ok\b")
    }

    default:
      return response(400, "worker.UnhandledPath\n")
  }
}

async function handleEmail(message: EmailMessage, env: Env, ctx: ExecutionContext) {
  let subject = message.headers.get("subject")
  switch (message.to) {
    case "msgauth@iio.ie":
      subject = encodeURIComponent(subject)
      let from = encodeURIComponent(message.from)
      let f = await fetch(`https://iio.ie/msgauthwait?from=${from}&id=${subject}`, {
        method: "POST",
        headers: {
          apikey: env.apikey,
        },
      })
      return
  }
}
