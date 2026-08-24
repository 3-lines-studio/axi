<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { Events } from '@wailsio/runtime';
  import { marked } from 'marked';
  import DOMPurify from 'dompurify';
  import { AXService } from '../bindings/github.com/3-lines-studio/axi';

  type Project = { id: string; name: string };
  type DirectoryRoot = { name: string; path: string };
  type Directory = { name: string; path: string; kind?: string; registered: boolean };
  type DirectoryResponse = { path?: string; parent?: string; roots?: DirectoryRoot[]; directories?: Directory[] };
  type Session = { id: string; project_id: string; title: string; updated_at: string };
  type StoredToolCall = { id: string; name: string; arguments: string };
  type StoredMessage = { role: string; content: string; tool_calls?: StoredToolCall[]; tool_call_id?: string };
  type Message = { kind: 'message'; role: 'user' | 'assistant'; content: string };
  type ToolItem = { kind: 'tool'; id: string; name: string; arguments: string; output: string; status: 'running' | 'done' | 'failed'; expanded: boolean };
  type ChatItem = Message | ToolItem;
  type SessionDetail = Session & { messages: StoredMessage[] };
  type AXEvent = { type: string; session_id?: string; run_id?: string; sequence?: number; id?: string; name?: string; arguments?: string; text?: string; output?: string };

  let baseUrl = $state('');
  let username = $state('');
  let password = $state('');
  let prompt = $state('');
  let connected = $state(false);
  let darkMode = $state(false);
  let runningSessions: string[] = $state([]);
  let loading = $state(false);
  let sidebarOpen = $state(false);
  let pickerOpen = $state(false);
  let pickerLoading = $state(false);
  let directoryPath = $state('');
  let directoryParent = $state('');
  let directoryRoots: DirectoryRoot[] = $state([]);
  let directories: Directory[] = $state([]);
  let directoryFilter = $state('');
  let projectName = $state('');
  let pendingDelete = $state<{ project: Project; session: Session }>();
  let error = $state('');
  let projects: Project[] = $state([]);
  let sessionsByProject: Record<string, Session[]> = $state({});
  let expandedProjects: string[] = $state([]);
  let loadedProjects: string[] = $state([]);
  let messages: ChatItem[] = $state([]);
  let transcriptsBySession: Record<string, ChatItem[]> = $state({});
  let projectId = $state('');
  let sessionId = $state('');
  let transcript = $state<HTMLElement>();
  let composer = $state<HTMLTextAreaElement>();

  onMount(() => {
    if (/Linux/.test(navigator.userAgent) && !/(Chrome|Chromium)/.test(navigator.userAgent)) {
      document.documentElement.dataset.engine = 'webkitgtk';
    }
    darkMode = localStorage.getItem('ax-theme') === 'dark';
    applyTheme();
    baseUrl = localStorage.getItem('ax-url') ?? '';
    username = localStorage.getItem('ax-username') ?? '';
    Events.On('ax:event', (event: { data: AXEvent }) => receive(event.data));
  });

  function toggleTheme(): void {
    darkMode = !darkMode;
    localStorage.setItem('ax-theme', darkMode ? 'dark' : 'light');
    applyTheme();
  }

  function applyTheme(): void {
    document.documentElement.dataset.theme = darkMode ? 'dark' : 'light';
  }

  async function connect(): Promise<void> {
    error = '';
    loading = true;
    try {
      await AXService.Connect(baseUrl, username, password);
      projects = await AXService.Projects() as Project[];
      const activeRuns = await AXService.ResumeRuns() as { session_id: string }[];
      runningSessions = activeRuns.map((run) => run.session_id);
      localStorage.setItem('ax-url', baseUrl.trim());
      localStorage.setItem('ax-username', username);
      connected = true;
      if (projects.length > 0) {
        const saved = JSON.parse(localStorage.getItem('ax-expanded-projects') ?? '[]') as string[];
        expandedProjects = saved.filter((id) => projects.some((project) => project.id === id));
        if (expandedProjects.length === 0) {
          expandedProjects = [projects[0].id];
        }
        await Promise.all(expandedProjects.map(loadSessions));
        const active = projects.some((item) => item.id === localStorage.getItem('ax-project'))
          ? localStorage.getItem('ax-project') ?? projects[0].id
          : projects[0].id;
        projectId = active;
        const latest = sessionsByProject[active]?.[0];
        if (latest) {
          await openSession(latest.id);
        }
      } else {
        await openPicker();
      }
    } catch (cause) {
      connected = false;
      error = messageFrom(cause);
    } finally {
      loading = false;
    }
  }

  async function loadSessions(id: string): Promise<void> {
    sessionsByProject[id] = await AXService.Sessions(id) as Session[];
    if (!loadedProjects.includes(id)) {
      loadedProjects = [...loadedProjects, id];
    }
  }

  async function toggleProject(id: string): Promise<void> {
    if (expandedProjects.includes(id)) {
      expandedProjects = expandedProjects.filter((item) => item !== id);
    } else {
      expandedProjects = [...expandedProjects, id];
      if (!loadedProjects.includes(id)) {
        await loadSessions(id);
      }
    }
    localStorage.setItem('ax-expanded-projects', JSON.stringify(expandedProjects));
  }

  async function openPicker(): Promise<void> {
    pickerOpen = true;
    directoryPath = '';
    directoryParent = '';
    directories = [];
    directoryFilter = '';
    projectName = '';
    await browse('');
  }

  async function browse(path: string): Promise<void> {
    pickerLoading = true;
    error = '';
    try {
      const result = await AXService.Directories(path) as DirectoryResponse;
      directoryRoots = result.roots ?? [];
      directoryPath = result.path ?? '';
      directoryParent = result.parent ?? '';
      directories = result.directories ?? [];
      directoryFilter = '';
      if (directoryPath) {
        projectName = directoryPath.split('/').filter(Boolean).at(-1) ?? '';
      }
    } catch (cause) {
      error = messageFrom(cause);
    } finally {
      pickerLoading = false;
    }
  }

  async function addProject(): Promise<void> {
    if (!directoryPath || !projectName.trim()) {
      return;
    }
    pickerLoading = true;
    error = '';
    try {
      const item = await AXService.AddProject(projectName.trim(), directoryPath) as Project;
      projects = await AXService.Projects() as Project[];
      pickerOpen = false;
      if (!expandedProjects.includes(item.id)) {
        expandedProjects = [...expandedProjects, item.id];
      }
      localStorage.setItem('ax-expanded-projects', JSON.stringify(expandedProjects));
      await loadSessions(item.id);
      await newSession(item.id);
    } catch (cause) {
      error = messageFrom(cause);
    } finally {
      pickerLoading = false;
    }
  }

  function visibleDirectories(): Directory[] {
    const filter = directoryFilter.trim().toLowerCase();
    if (!filter) {
      return directories;
    }
    return directories.filter((item) => item.name.toLowerCase().includes(filter));
  }

  async function newSession(targetProject = projectId): Promise<void> {
    if (!targetProject) {
      return;
    }
    error = '';
    try {
      const item = await AXService.NewSession(targetProject) as SessionDetail;
      projectId = targetProject;
      sessionId = item.id;
      messages = [];
      transcriptsBySession[item.id] = messages;
      sessionsByProject[targetProject] = [item, ...(sessionsByProject[targetProject] ?? [])];
      localStorage.setItem('ax-project', targetProject);
      sidebarOpen = false;
      await tick();
      composer?.focus();
    } catch (cause) {
      error = messageFrom(cause);
    }
  }

  function requestDelete(project: Project, session: Session): void {
    if (!isRunning(session.id)) {
      pendingDelete = { project, session };
    }
  }

  async function deleteSession(): Promise<void> {
    if (!pendingDelete) {
      return;
    }
    const { project, session } = pendingDelete;
    pendingDelete = undefined;
    error = '';
    try {
      await AXService.DeleteSession(session.id);
      sessionsByProject[project.id] = (sessionsByProject[project.id] ?? []).filter((item) => item.id !== session.id);
      if (sessionId === session.id) {
        sessionId = '';
        messages = [];
      }
    } catch (cause) {
      error = messageFrom(cause);
    }
  }

  async function openSession(id: string): Promise<void> {
    loading = true;
    error = '';
    try {
      const item = await AXService.OpenSession(id) as SessionDetail;
      projectId = item.project_id;
      sessionId = item.id;
      const restored = restoreTranscript(item.messages ?? []);
      const cached = transcriptsBySession[id];
      messages = cached && isRunning(id) ? cached : restored;
      transcriptsBySession[id] = messages;
      localStorage.setItem('ax-project', item.project_id);
      sidebarOpen = false;
      await scrollToEnd();
    } catch (cause) {
      error = messageFrom(cause);
    } finally {
      loading = false;
    }
  }

  async function send(): Promise<void> {
    const text = prompt.trim();
    if (!text || isRunning(sessionId)) {
      return;
    }
    if (!sessionId) {
      await newSession();
      if (!sessionId) {
        return;
      }
    }

    const targetSession = sessionId;
    messages.push({ kind: 'message', role: 'user', content: text });
    messages.push({ kind: 'message', role: 'assistant', content: '' });
    prompt = '';
    error = '';
    runningSessions = [...runningSessions, targetSession];
    transcriptsBySession[targetSession] = messages;
    await scrollToEnd();

    try {
      await AXService.Send(targetSession, text);
    } catch (cause) {
      transcriptsBySession[targetSession]?.pop();
      runningSessions = runningSessions.filter((id) => id !== targetSession);
      error = messageFrom(cause);
    }
  }

  async function cancel(): Promise<void> {
    try {
      await AXService.Cancel(sessionId);
    } catch (cause) {
      error = messageFrom(cause);
    }
  }

  function receive(event: AXEvent): void {
    const targetSession = event.session_id;
    if (!targetSession) {
      return;
    }
    const target = transcriptsBySession[targetSession] ?? [];
    transcriptsBySession[targetSession] = target;
    if (targetSession === sessionId && messages !== target) {
      messages = target;
    }
    if (event.type === 'delta') {
      let message = target.at(-1);
      if (message?.kind !== 'message' || message.role !== 'assistant') {
        message = { kind: 'message', role: 'assistant', content: '' };
        target.push(message);
      }
      message.content += event.text ?? '';
      if (targetSession === sessionId) {
        void scrollToEnd();
      }
      return;
    }
    if (event.type === 'tool_start' && event.id && event.name) {
      const last = target.at(-1);
      if (last?.kind === 'message' && last.role === 'assistant' && last.content === '') {
        target.pop();
      }
      target.push({ kind: 'tool', id: event.id, name: event.name, arguments: event.arguments ?? '', output: '', status: 'running', expanded: true });
      if (targetSession === sessionId) {
        void scrollToEnd();
      }
      return;
    }
    if (event.type === 'tool_delta' && event.id) {
      const item = findTool(target, event.id);
      if (item) {
        item.output += event.text ?? '';
      }
      return;
    }
    if (event.type === 'tool_result' && event.id) {
      const item = findTool(target, event.id);
      if (item) {
        item.output = event.output ?? '';
        item.status = item.output.toLowerCase().startsWith('error:') ? 'failed' : 'done';
        item.expanded = item.status === 'failed';
      }
      return;
    }
    if (event.type === 'failure' && targetSession === sessionId) {
      error = event.text ?? 'AX failed';
    }
    const last = target.at(-1);
    if (event.type === 'cancelled' && last?.kind === 'message' && last.content === '') {
      target.pop();
    }
    if (event.type === 'done' || event.type === 'failure' || event.type === 'cancelled') {
      runningSessions = runningSessions.filter((id) => id !== targetSession);
      void Promise.all(loadedProjects.map(loadSessions));
      if (targetSession === sessionId) {
        void tick().then(() => composer?.focus());
      }
    }
  }

  function findTool(items: ChatItem[], id: string): ToolItem | undefined {
    return items.find((item): item is ToolItem => item.kind === 'tool' && item.id === id);
  }

  function isRunning(id: string): boolean {
    return runningSessions.includes(id);
  }

  function restoreTranscript(stored: StoredMessage[]): ChatItem[] {
    const items: ChatItem[] = [];
    const tools = new Map<string, ToolItem>();
    for (const message of stored) {
      if ((message.role === 'user' || message.role === 'assistant') && message.content) {
        items.push({ kind: 'message', role: message.role, content: message.content });
      }
      for (const call of message.tool_calls ?? []) {
        const item: ToolItem = { kind: 'tool', id: call.id, name: call.name, arguments: call.arguments, output: '', status: 'done', expanded: false };
        items.push(item);
        tools.set(call.id, item);
      }
      if (message.role === 'tool' && message.tool_call_id) {
        const item = tools.get(message.tool_call_id);
        if (item) {
          item.output = message.content;
          item.status = message.content.toLowerCase().startsWith('error:') ? 'failed' : 'done';
          item.expanded = item.status === 'failed';
        }
      }
    }
    return items;
  }

  function toolSummary(item: ToolItem): string {
    try {
      const values = Object.values(JSON.parse(item.arguments) as Record<string, unknown>);
      const value = values.find((entry) => typeof entry === 'string');
      return typeof value === 'string' ? value.replace(/\s+/g, ' ').slice(0, 80) : '';
    } catch {
      return item.arguments.slice(0, 80);
    }
  }

  function prettyArguments(value: string): string {
    try {
      return JSON.stringify(JSON.parse(value), null, 2);
    } catch {
      return value;
    }
  }

  async function refreshSessions(): Promise<void> {
    if (projectId) {
      await loadSessions(projectId);
    }
  }

  function keydown(event: KeyboardEvent): void {
    if (event.key !== 'Enter' || event.shiftKey || event.isComposing) {
      return;
    }
    event.preventDefault();
    void send();
  }

  function render(text: string): string {
    return DOMPurify.sanitize(marked.parse(text, { async: false }) as string);
  }

  function messageFrom(cause: unknown): string {
    return cause instanceof Error ? cause.message : String(cause);
  }

  async function scrollToEnd(): Promise<void> {
    await tick();
    transcript?.scrollTo({ top: transcript.scrollHeight });
  }
</script>

{#if !connected}
  <main class="connect-shell">
    <form class="connect-card" onsubmit={(event) => { event.preventDefault(); void connect(); }}>
      <button class="theme-toggle connect-theme" type="button" onclick={toggleTheme} aria-label={darkMode ? 'Use light mode' : 'Use dark mode'} title={darkMode ? 'Use light mode' : 'Use dark mode'}>
        {#if darkMode}<svg viewBox="0 0 20 20" aria-hidden="true"><circle cx="10" cy="10" r="3.25"/><path d="M10 1.5v2M10 16.5v2M1.5 10h2M16.5 10h2M4 4l1.4 1.4M14.6 14.6 16 16M16 4l-1.4 1.4M5.4 14.6 4 16"/></svg>{:else}<svg viewBox="0 0 20 20" aria-hidden="true"><path d="M16.8 12.7A7 7 0 0 1 7.3 3.2 7 7 0 1 0 16.8 12.7Z"/></svg>{/if}
      </button>
      <div class="mark">AX</div>
      <div><h1>Connect to Axis</h1><p>Enter your Axis server URL.</p></div>
      <label>Server URL<input bind:value={baseUrl} type="url" inputmode="url" placeholder="https://ax.example.com" required /></label>
      <label>Username<input bind:value={username} autocomplete="username" /></label>
      <label>Password<input bind:value={password} type="password" autocomplete="current-password" /></label>
      {#if error}<p class="error" role="alert">{error}</p>{/if}
      <button class="primary" type="submit" disabled={loading}>{loading ? 'Connecting…' : 'Connect'}</button>
    </form>
  </main>
{:else}
  <main class="app-shell">
    <header>
      <button class="mobile-menu" onclick={() => sidebarOpen = !sidebarOpen} aria-label="Sessions">☰</button>
      <div class="mark small">AX</div>
      <strong class="active-project">{projects.find((project) => project.id === projectId)?.name ?? 'No project selected'}</strong>
      <span class="server-url">{baseUrl}</span>
      <button class="theme-toggle" onclick={toggleTheme} aria-label={darkMode ? 'Use light mode' : 'Use dark mode'} title={darkMode ? 'Use light mode' : 'Use dark mode'}>
        {#if darkMode}<svg viewBox="0 0 20 20" aria-hidden="true"><circle cx="10" cy="10" r="3.25"/><path d="M10 1.5v2M10 16.5v2M1.5 10h2M16.5 10h2M4 4l1.4 1.4M14.6 14.6 16 16M16 4l-1.4 1.4M5.4 14.6 4 16"/></svg>{:else}<svg viewBox="0 0 20 20" aria-hidden="true"><path d="M16.8 12.7A7 7 0 0 1 7.3 3.2 7 7 0 1 0 16.8 12.7Z"/></svg>{/if}
      </button>
      <button class="quiet server-action" onclick={() => { connected = false; messages = []; }} aria-label="Change server" title="Change server"><svg viewBox="0 0 20 20" aria-hidden="true"><path d="M4 5h12M4 10h12M4 15h12"/><circle cx="7" cy="5" r="1"/><circle cx="13" cy="10" r="1"/><circle cx="8" cy="15" r="1"/></svg><span>Change server</span></button>
    </header>

    <div class="workspace">
      <aside class:open={sidebarOpen}>
        <button class="add-project top" onclick={() => void openPicker()}>+ Add directory</button>
        <nav class="projects">
          {#each projects as project}
            <section class:current={project.id === projectId}>
              <div class="project-row">
                <button class="project-toggle" onclick={() => void toggleProject(project.id)} aria-expanded={expandedProjects.includes(project.id)}>
                  <svg class:expanded={expandedProjects.includes(project.id)} viewBox="0 0 16 16" aria-hidden="true"><path d="m6 3 5 5-5 5" /></svg>
                  <strong>{project.name}</strong>
                </button>
                <button class="project-new" onclick={() => void newSession(project.id)} aria-label={`New chat in ${project.name}`} title="New chat">
                  <svg viewBox="0 0 20 20" aria-hidden="true"><path d="M4 3.5h8a2 2 0 0 1 2 2v4M4 3.5a2 2 0 0 0-2 2v7a2 2 0 0 0 2 2h3l3 2.5v-2.5h2M15 11v6m-3-3h6" /></svg>
                </button>
              </div>
              {#if expandedProjects.includes(project.id)}
                <div class="project-sessions">
                  {#each sessionsByProject[project.id] ?? [] as session}
                    <div class="session-row" class:active={session.id === sessionId}>
                      <button class="session-open" onclick={() => void openSession(session.id)}><span>{session.title}</span>{#if isRunning(session.id)}<i class="session-running" aria-label="Running"></i>{/if}</button>
                      <button class="session-delete" onclick={() => requestDelete(project, session)} disabled={isRunning(session.id)} aria-label={`Delete ${session.title}`} title="Delete chat">
                        <svg viewBox="0 0 20 20" aria-hidden="true"><path d="M4 6h12M8 3h4l1 3H7l1-3Zm-2 3 1 11h6l1-11M9 9v5m2-5v5" /></svg>
                      </button>
                    </div>
                  {:else}
                    <p>No chats yet</p>
                  {/each}
                </div>
              {/if}
            </section>
          {/each}
        </nav>
      </aside>
      {#if sidebarOpen}<button class="scrim" onclick={() => sidebarOpen = false} aria-label="Close sessions"></button>{/if}

      <section class="chat">
        <div class="transcript" bind:this={transcript} aria-live="polite">
          {#if messages.length === 0 && !loading}
            <div class="empty"><h1>What can I help with?</h1><p>Messages run in the selected project.</p></div>
          {/if}
          {#each messages as item}
            {#if item.kind === 'message'}
              <article class:assistant={item.role === 'assistant'} class:user={item.role === 'user'}>
                <div class="role">{item.role === 'assistant' ? 'AX' : 'You'}</div>
                <div class="message-body">
                  {#if item.content}{@html render(item.content)}{:else}<span class="cursor"></span>{/if}
                </div>
              </article>
            {:else}
              <div class="tool-card" class:failed={item.status === 'failed'} class:running={item.status === 'running'}>
                <button class="tool-head" onclick={() => item.expanded = !item.expanded} aria-expanded={item.expanded}>
                  <span class="tool-icon">
                    {#if item.name === 'bash' || item.name === 'bashx'}
                      <svg viewBox="0 0 20 20" aria-hidden="true"><path d="m4 6 3 3-3 3m5 1h6M3 3h14v14H3z" /></svg>
                    {:else if item.name === 'read' || item.name === 'write' || item.name === 'edit'}
                      <svg viewBox="0 0 20 20" aria-hidden="true"><path d="M5 2.5h7l3 3v12H5zM12 2.5v3h3M8 9h4m-4 3h4" /></svg>
                    {:else}
                      <svg viewBox="0 0 20 20" aria-hidden="true"><path d="m12 3 1 3 3 1-2 2 1 3-3-1-2 3-2-3-3 1 1-3-2-2 3-1 1-3 2 2z" /></svg>
                    {/if}
                  </span>
                  <strong>{item.name}</strong>
                  <span class="tool-summary">{toolSummary(item)}</span>
                  <span class="tool-status">{item.status === 'running' ? 'Running' : item.status === 'failed' ? 'Failed' : 'Done'}</span>
                  <svg class="tool-chevron" class:expanded={item.expanded} viewBox="0 0 16 16" aria-hidden="true"><path d="m6 3 5 5-5 5" /></svg>
                </button>
                {#if item.expanded}
                  <div class="tool-detail">
                    {#if item.arguments}
                      <div class="tool-section"><span>Arguments</span><pre>{prettyArguments(item.arguments)}</pre></div>
                    {/if}
                    {#if item.output}
                      <div class="tool-section">
                        <div class="tool-section-head"><span>Output</span><button onclick={() => void navigator.clipboard.writeText(item.output)}>Copy</button></div>
                        <pre>{item.output}</pre>
                      </div>
                    {:else if item.status === 'running'}
                      <div class="tool-progress"><span></span>Waiting for output</div>
                    {/if}
                  </div>
                {/if}
              </div>
            {/if}
          {/each}
        </div>

        <footer>
          {#if error}<p class="error footer-error" role="alert">{error}</p>{/if}
          <div class="composer">
            <textarea bind:this={composer} bind:value={prompt} onkeydown={keydown} rows="1" placeholder="Message AX" aria-label="Message AX"></textarea>
            {#if isRunning(sessionId)}
              <button class="stop" onclick={() => void cancel()} aria-label="Stop">■</button>
            {:else}
              <button class="send" onclick={() => void send()} disabled={!prompt.trim()} aria-label="Send">↑</button>
            {/if}
          </div>
          <p class="hint">Enter to send · Shift+Enter for a new line</p>
        </footer>
      </section>
    </div>
  </main>

  {#if pendingDelete}
    <div class="modal-backdrop delete-backdrop" role="presentation">
      <div class="delete-dialog" role="alertdialog" aria-modal="true" aria-labelledby="delete-title" aria-describedby="delete-description">
        <div class="delete-icon">
          <svg viewBox="0 0 20 20" aria-hidden="true"><path d="M4 6h12M8 3h4l1 3H7l1-3Zm-2 3 1 11h6l1-11M9 9v5m2-5v5" /></svg>
        </div>
        <div>
          <h2 id="delete-title">Delete chat?</h2>
          <p id="delete-description"><strong>{pendingDelete.session.title}</strong> and its full history will be permanently removed.</p>
        </div>
        <div class="delete-actions">
          <button class="quiet-action" onclick={() => pendingDelete = undefined}>Cancel</button>
          <button class="delete-action" onclick={() => void deleteSession()}>Delete chat</button>
        </div>
      </div>
    </div>
  {/if}

  {#if pickerOpen}
    <div class="modal-backdrop" role="presentation">
      <div class="directory-picker" role="dialog" aria-modal="true" aria-labelledby="picker-title">
        <div class="picker-head">
          <div><h2 id="picker-title">Add directory</h2><p>{directoryPath || 'Choose a location'}</p></div>
          <button class="close" onclick={() => pickerOpen = false} aria-label="Close">×</button>
        </div>

        {#if directoryPath}
          <input class="folder-filter" bind:value={directoryFilter} type="search" placeholder="Filter folders" aria-label="Filter folders" />
        {/if}

        <div class="directory-list">
          {#if !directoryPath}
            {#each directoryRoots as root}
              <button onclick={() => void browse(root.path)}><span class="folder-icon">□</span><span><strong>{root.name}</strong><small>{root.path}</small></span><b>›</b></button>
            {/each}
          {:else}
            {#if directoryParent}<button onclick={() => void browse(directoryParent)}><span class="folder-icon">↑</span><span><strong>Parent directory</strong></span></button>{/if}
            {#each visibleDirectories() as directory}
              <button onclick={() => void browse(directory.path)}>
                <span class="folder-icon">□</span>
                <span><strong>{directory.name}</strong><small>{directory.registered ? 'Already added' : directory.kind ?? ''}</small></span>
                <b>›</b>
              </button>
            {/each}
          {/if}
          {#if pickerLoading}<p class="picker-status">Loading…</p>{/if}
        </div>

        {#if directoryPath}
          <div class="picker-form">
            <label>Project name<input bind:value={projectName} maxlength="80" /></label>
            {#if error}<p class="error" role="alert">{error}</p>{/if}
            <div class="picker-actions">
              <button class="quiet-action" onclick={() => pickerOpen = false}>Cancel</button>
              <button class="primary-action" onclick={() => void addProject()} disabled={pickerLoading || !projectName.trim()}>Add this directory</button>
            </div>
          </div>
        {/if}
      </div>
    </div>
  {/if}
{/if}
