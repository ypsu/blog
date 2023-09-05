// source code for the cloudflare worker.
// run `wrangler dev -r` do develop.
// run `wrangler deploy` to deploy.

export default {
  fetch
}

function response(code: number, message: string) {
  return new Response(message, {
    status: code
  })
}

async function fetch(request: Request, env: Env, ctx: ExecutionContext): Promise < Response > {
  let method = request.method
  let path = (new URL(request.url)).pathname
  let params = (new URL(request.url)).searchParams
  if (env.devenv == 0 && request.headers.get('cfkey') != env.cfkey) {
    return response(403, 'unathorized')
  }

  let value, list, r
  switch (true) {
    case path == '/kv' && method == 'GET':
      let value = await env.data.get(params.get('key'))
      if (value == null) return response(404, 'key not found')
      return response(200, value)
    case path == '/kv' && method == 'PUT':
      await env.data.put(params.get('key'), await request.text())
      return response(200, 'ok')
    case path == '/kv' && method == 'DELETE':
      await env.data.delete(params.get('key'))
      return response(200, 'ok')
    case path == '/kvlist':
      list = await env.data.list({
        prefix: params.get('prefix')
      })
      if (list == null) return response(500, 'list failed')
      if (!list.list_complete) console.log('/kvlist result too long.')
      r = ''
      for (let key of list.keys) r += `${key.name}\n`
      return response(200, r)
    case path == '/kvall':
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
