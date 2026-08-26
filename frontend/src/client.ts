export type ClientEvent = {
  type: string
  session_id?: string
  run_id?: string
  sequence?: number
  id?: string
  name?: string
  arguments?: string
  text?: string
  output?: string
  media_type?: string
  size?: number
  input?: number
  usage?: { input: number; output: number; cached_input: number }
}

type Listener = (event: ClientEvent) => void

type Run = { id: string; session_id: string; status: string }

const listeners = new Set<Listener>()
const runs = new Map<string, string>()

export const browser = !('_wails' in window) && !('wails' in window)

function emit(event: ClientEvent): void {
  for (const listener of listeners) {
    listener(event)
  }
}

async function request(path: string, options?: RequestInit): Promise<any> {
  const response = await fetch(path, options)
  if (!response.ok) {
    throw new Error((await response.text()).trim() || response.statusText)
  }
  if (response.status === 204) {
    return
  }
  return response.json()
}

async function attach(session: string, run: string): Promise<void> {
  runs.set(session, run)
  let sequence = 0
  for (let attempt = 0; attempt < 4; attempt++) {
    try {
      const response = await fetch(`/runs/${encodeURIComponent(run)}/events?after=${sequence}`)
      if (!response.ok || !response.body) {
        throw new Error((await response.text()).trim() || response.statusText)
      }
      const reader = response.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''
      let type = 'message'
      for (;;) {
        const result = await reader.read()
        buffer += decoder.decode(result.value, { stream: !result.done })
        const lines = buffer.split('\n')
        buffer = lines.pop() ?? ''
        for (const line of lines) {
          if (line.startsWith('id: ')) {
            sequence = Number(line.slice(4))
            continue
          }
          if (line.startsWith('event: ')) {
            type = line.slice(7)
            continue
          }
          if (!line.startsWith('data: ')) {
            continue
          }
          const value = line.slice(6)
          const data = value === 'null' ? {} : JSON.parse(value)
          const event = typeof data === 'string' ? { text: data } : data
          emit({ ...event, type, session_id: session, run_id: run, sequence })
          if (type === 'done' || type === 'failure') {
            runs.delete(session)
            return
          }
          type = 'message'
        }
        if (result.done) {
          throw new Error('stream ended')
        }
      }
    } catch {
      if (attempt < 3) {
        await new Promise((resolve) => setTimeout(resolve, 500 * (attempt + 1)))
      }
    }
  }
  runs.delete(session)
  emit({ type: 'failure', session_id: session, run_id: run, text: 'Connection to the run was lost.' })
}

const browserClient = {
  async Connect(_baseUrl: string, _username: string, _password: string): Promise<void> {},
  Projects: () => request('/api/projects'),
  Bots: () => request('/api/bots'),
  SaveBot: (bot: { id: string }) => request(bot.id ? `/api/bots/${encodeURIComponent(bot.id)}` : '/api/bots', { method: bot.id ? 'PUT' : 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(bot) }),
  DeleteBot: (bot: string) => request(`/api/bots/${encodeURIComponent(bot)}`, { method: 'DELETE' }),
  Connectors: () => request('/api/connectors'),
  SaveConnector: (connector: { id: string }) => request(connector.id ? `/api/connectors/${encodeURIComponent(connector.id)}` : '/api/connectors', { method: connector.id ? 'PUT' : 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(connector) }),
  DeleteConnector: (connector: string) => request(`/api/connectors/${encodeURIComponent(connector)}`, { method: 'DELETE' }),
  Sessions: (project: string) => request(`/api/projects/${encodeURIComponent(project)}/sessions`),
  Directories: (path: string) => request(`/api/directories?path=${encodeURIComponent(path)}`),
  Commands: () => request('/api/commands'),
  ProjectFiles: (project: string, query: string) => request(`/api/projects/${encodeURIComponent(project)}/files?q=${encodeURIComponent(query)}`),
  AddProject: (name: string, path: string) => request('/api/projects', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name, path }) }),
  DeleteProject: (project: string) => request(`/api/projects/${encodeURIComponent(project)}`, { method: 'DELETE' }),
  NewSession: (project: string, bot: string) => request(`/api/projects/${encodeURIComponent(project)}/sessions`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ bot_id: bot }) }),
  DeleteSession: (session: string) => request(`/api/sessions/${encodeURIComponent(session)}`, { method: 'DELETE' }),
  ArtifactSource: (session: string, artifact: string) => Promise.resolve(`/api/sessions/${encodeURIComponent(session)}/artifacts/${encodeURIComponent(artifact)}`),
  OpenSession: (session: string) => request(`/api/sessions/${encodeURIComponent(session)}`),
  async ResumeRuns(): Promise<Run[]> {
    const active = await request('/api/runs') as Run[]
    for (const run of active) {
      void attach(run.session_id, run.id)
    }
    return active
  },
  async Send(session: string, prompt: string): Promise<void> {
    const body = new URLSearchParams({ prompt })
    const result = await request(`/sessions/${encodeURIComponent(session)}/messages`, { method: 'POST', body })
    void attach(session, result.run_id)
  },
  async Cancel(session: string): Promise<void> {
    const run = runs.get(session)
    if (!run) {
      return
    }
    await request(`/runs/${encodeURIComponent(run)}/cancel`, { method: 'POST' })
  },
  async Steer(session: string, text: string): Promise<void> {
    const run = runs.get(session)
    if (!run) {
      throw new Error('this chat is not running')
    }
    await request(`/runs/${encodeURIComponent(run)}/steer`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ text }) })
  }
}

const nativeClient = browser ? undefined : (await import('../bindings/github.com/3-lines-studio/axi')).AXService

export const client = browser ? browserClient : nativeClient!

export async function subscribe(listener: Listener): Promise<void> {
  listeners.add(listener)
  if (browser) {
    return
  }
  const { Events } = await import('@wailsio/runtime')
  Events.On('ax:event', (event: { data: ClientEvent }) => listener(event.data))
}
