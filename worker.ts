// source code for the cloudflare worker.
// run `wrangler dev -r` do develop.
// run `wrangler deploy` to deploy.
// run `wrangler tail` for the prod logs.
// run `rm -rf ~/.npm` to get a new version of wrangler.
// use https://dash.cloudflare.com/3b11ecb3c60ca956441f147edbd895c2/workers/kv/namespaces to manage the comments manually.

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
  if (env.devenv == 0 && request.headers.get('apikey') != env.apikey) {
    return response(403, 'unathorized')
  }

  let value, list, r
  switch (true) {
    case path == '/api/kv' && method == 'GET':
      let value = await env.data.get(params.get('key'))
      if (value == null) return response(404, 'key not found')
      return response(200, value)

    case path == '/api/kv' && method == 'PUT':
      await env.data.put(params.get('key'), await request.text())
      return response(200, 'ok')

    case path == '/api/kv' && method == 'DELETE':
      await env.data.delete(params.get('key'))
      return response(200, 'ok')

    case path == '/api/kvlist':
      list = await env.data.list({
        prefix: params.get('prefix')
      })
      if (list == null) return response(500, 'list failed')
      if (!list.list_complete) console.log('/kvlist result too long.')
      r = ''
      for (let key of list.keys) r += `${key.name}\n`
      return response(200, r)

    case path == '/api/kvall':
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
      let f = await fetch(`https://iio.ie/msgauthwait?from=${from}&id=${subject}`, {
        method: 'POST',
        headers: {
          'apikey': env.apikey,
        },
      })
      return
  }
}
