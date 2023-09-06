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
    status: code
  })
}

async function handleFetch(request: Request, env: Env, ctx: ExecutionContext): Promise < Response > {
  let method = request.method
  let path = (new URL(request.url)).pathname
  let params = (new URL(request.url)).searchParams
  if (path != '/cf/test' && env.devenv == 0 && request.headers.get('cfkey') != env.cfkey) {
    return response(403, 'unathorized')
  }

  let value, list, r
  switch (true) {
    case path == '/cf/kv' && method == 'GET':
      let value = await env.data.get(params.get('key'))
      if (value == null) return response(404, 'key not found')
      return response(200, value)

    case path == '/cf/kv' && method == 'PUT':
      await env.data.put(params.get('key'), await request.text())
      return response(200, 'ok')

    case path == '/cf/kv' && method == 'DELETE':
      await env.data.delete(params.get('key'))
      return response(200, 'ok')

    case path == '/cf/kvlist':
      list = await env.data.list({
        prefix: params.get('prefix')
      })
      if (list == null) return response(500, 'list failed')
      if (!list.list_complete) console.log('/kvlist result too long.')
      r = ''
      for (let key of list.keys) r += `${key.name}\n`
      return response(200, r)

    case path == '/cf/kvall':
      list = await env.data.list({
        prefix: params.get('prefix')
      })
      if (list == null) return response(500, 'list failed')
      if (!list.list_complete) console.log('/kvall result too long.')
      let fetches = []
      for (let key of list.keys) fetches.push(env.data.get(key.name))
      r = ''
      for (let f of fetches) r += await f
      return response(200, r)

    default:
      return response(400, 'bad path')
  }
}

async function handleEmail(message: EmailMessage, env: Env, ctx: ExecutionContext) {
  let subject = message.headers.get('subject')
  switch (message.to) {
    case 'msgauth@iio.ie':
      subject = encodeURIComponent(subject)
      let from = encodeURIComponent(message.from)
      let f = await fetch(`https://iio.fly.dev/msgauthwait?login&id=${subject}&from=${from}`, {
        method: 'POST',
        headers: {
          'cfkey': env.cfkey,
        },
      })
      return
  }
}
