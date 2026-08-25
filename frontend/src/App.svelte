<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { marked } from 'marked';
  import DOMPurify from 'dompurify';
  import { ArrowDown, ArrowUp, Bot as BotIcon, ChevronRight, Copy, FileText, Folder, FolderMinus, FolderUp, Menu, MessageSquarePlus, Moon, Paperclip, Plug, Plus, Power, Settings, SlidersHorizontal, Sun, Terminal, Trash2, Wrench, X } from '@lucide/svelte';
  import { browser, client, subscribe } from './client';

  type Project = { id: string; name: string };
  type Bot = { id: string; name: string; prompt: string; tools: string[]; workspace_root: string; skill_root?: string; model?: string };
  type Connector = { id: string; name: string; type: string; bot_id: string; project_id: string; enabled: boolean; status?: string; error?: string; restarts?: number };
  type DirectoryRoot = { name: string; path: string };
  type Directory = { name: string; path: string; kind?: string; registered: boolean };
  type DirectoryResponse = { path?: string; parent?: string; roots?: DirectoryRoot[]; directories?: Directory[] };
  type Session = { id: string; project_id: string; bot_id: string; title: string; updated_at: string };
  type StoredToolCall = { id: string; name: string; arguments: string };
  type StoredMessage = { role: string; content: string; tool_calls?: StoredToolCall[]; tool_call_id?: string };
  type StoredArtifact = { id: string; name: string; media_type: string; size: number };
  type Message = { kind: 'message'; role: 'user' | 'assistant'; content: string };
  type ToolItem = { kind: 'tool'; id: string; name: string; arguments: string; output: string; status: 'running' | 'done' | 'failed'; expanded: boolean };
  type ArtifactItem = { kind: 'artifact'; id: string; name: string; media_type: string; size: number; session_id: string; source?: string };
  type ChatItem = Message | ToolItem | ArtifactItem;
  type SessionDetail = Session & { messages: StoredMessage[]; artifacts: StoredArtifact[] };
  type AXEvent = { type: string; session_id?: string; run_id?: string; sequence?: number; id?: string; name?: string; arguments?: string; text?: string; output?: string; media_type?: string; size?: number };

  let baseUrl = $state('');
  let username = $state('');
  let password = $state('');
  let setupChecking = $state(browser);
  let setupRequired = $state(false);
  let setupExisting = $state(false);
  let setupAPIKey = $state('');
  let setupModel = $state('gpt-4.1-mini');
  let setupBase = $state('');
  let prompt = $state('');
  let connected = $state(false);
  let stopped = $state(false);
  let darkMode = $state(false);
  let runningSessions: string[] = $state([]);
  let loading = $state(false);
  let sidebarOpen = $state(false);
  let pickerOpen = $state(false);
  let pickerLoading = $state(false);
  let pickerTarget: 'project' | 'workspace' | 'skills' = $state('project');
  let directoryPath = $state('');
  let directoryParent = $state('');
  let directoryRoots: DirectoryRoot[] = $state([]);
  let directories: Directory[] = $state([]);
  let directoryFilter = $state('');
  let projectName = $state('');
  let pendingDelete = $state<{ project: Project; session: Session }>();
  let pendingProjectDelete = $state<Project>();
  let error = $state('');
  let projects: Project[] = $state([]);
  let bots: Bot[] = $state([]);
  let botId = $state('');
  let editingBot = $state<Bot>();
  let connectors: Connector[] = $state([]);
  let editingConnector = $state<Connector>();
  let botTools = $state('');
  let sessionsByProject: Record<string, Session[]> = $state({});
  let expandedProjects: string[] = $state([]);
  let loadedProjects: string[] = $state([]);
  let messages: ChatItem[] = $state([]);
  let transcriptsBySession: Record<string, ChatItem[]> = $state({});
  let projectId = $state('');
  let sessionId = $state('');
  let transcript = $state<HTMLElement>();
  let composer = $state<HTMLTextAreaElement>();
  let atBottom = $state(true);

  onMount(async () => {
    if (/Linux/.test(navigator.userAgent) && !/(Chrome|Chromium)/.test(navigator.userAgent)) {
      document.documentElement.dataset.engine = 'webkitgtk';
    }
    darkMode = localStorage.getItem('ax-theme') === 'dark';
    applyTheme();
    baseUrl = localStorage.getItem('ax-url') ?? '';
    username = localStorage.getItem('ax-username') ?? '';
    await subscribe(receive);
    if (browser) {
      try {
        const response = await fetch('/api/setup');
        if (response.ok) {
          const setup = await response.json() as { configured: boolean };
          setupRequired = !setup.configured;
        }
      } catch {
        setupRequired = false;
      } finally {
        setupChecking = false;
      }
      if (!setupRequired) {
        await connect();
      }
    }
  });

  function openSetup(): void {
    setupExisting = true;
    setupRequired = true;
    error = '';
  }

  async function quitAxi(): Promise<void> {
    if (!browser) {
      return;
    }
    try {
      await fetch('/api/local/quit', { method: 'POST' });
    } finally {
      stopped = true;
      connected = false;
    }
  }

  async function saveSetup(): Promise<void> {
    error = '';
    loading = true;
    try {
      const response = await fetch('/api/setup', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ api_key: setupAPIKey, model: setupModel, base: setupBase })
      });
      if (!response.ok) {
        throw new Error(await response.text());
      }
      setupAPIKey = '';
      setupRequired = false;
      await connect();
    } catch (cause) {
      error = messageFrom(cause);
    } finally {
      loading = false;
    }
  }

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
      await client.Connect(baseUrl, username, password);
      projects = await client.Projects() as Project[];
      bots = await client.Bots() as Bot[];
      connectors = await client.Connectors() as Connector[];
      botId = bots.some((bot) => bot.id === localStorage.getItem('ax-bot')) ? localStorage.getItem('ax-bot') ?? '' : bots[0]?.id ?? '';
      const activeRuns = await client.ResumeRuns() as { session_id: string }[];
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
    sessionsByProject[id] = await client.Sessions(id) as Session[];
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

  async function openPicker(target: 'project' | 'workspace' | 'skills' = 'project'): Promise<void> {
    pickerTarget = target;
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
      const result = await client.Directories(path) as DirectoryResponse;
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

  function chooseBotDirectory(): void {
    if (!editingBot || !directoryPath) {
      return;
    }
    if (pickerTarget === 'workspace') {
      editingBot.workspace_root = directoryPath;
    } else {
      editingBot.skill_root = directoryPath;
    }
    pickerOpen = false;
  }

  async function addProject(): Promise<void> {
    if (!directoryPath || !projectName.trim()) {
      return;
    }
    pickerLoading = true;
    error = '';
    try {
      const item = await client.AddProject(projectName.trim(), directoryPath) as Project;
      projects = await client.Projects() as Project[];
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

  function selectBot(id: string): void {
    if (botId === id) {
      return;
    }
    botId = id;
    sessionId = '';
    messages = [];
    localStorage.setItem('ax-bot', id);
  }

  function editBot(item?: Bot): void {
    editingBot = item ? { ...item, tools: [...item.tools] } : { id: '', name: '', prompt: '', tools: [], workspace_root: '' };
    botTools = item?.tools.join(' ') ?? '';
  }

  async function saveBot(): Promise<void> {
    if (!editingBot) {
      return;
    }
    error = '';
    try {
      const item = await client.SaveBot({ ...editingBot, tools: botTools.split(/\s+/).filter(Boolean) }) as Bot;
      const index = bots.findIndex((bot) => bot.id === item.id);
      bots = index === -1 ? [...bots, item] : bots.map((bot) => bot.id === item.id ? item : bot);
      selectBot(item.id);
      editingBot = undefined;
    } catch (cause) {
      error = messageFrom(cause);
    }
  }

  async function deleteBot(): Promise<void> {
    if (!editingBot?.id) {
      return;
    }
    error = '';
    try {
      await client.DeleteBot(editingBot.id);
      bots = bots.filter((bot) => bot.id !== editingBot?.id);
      if (botId === editingBot.id) {
        selectBot(bots[0]?.id ?? '');
      }
      editingBot = undefined;
    } catch (cause) {
      error = messageFrom(cause);
    }
  }

  function editConnector(item?: Connector): void {
    editingConnector = item ? { ...item } : { id: '', name: '', type: 'slack', bot_id: botId || bots[0]?.id || '', project_id: projectId || projects[0]?.id || '', enabled: false };
  }

  async function saveConnector(): Promise<void> {
    if (!editingConnector) {
      return;
    }
    error = '';
    try {
      const item = await client.SaveConnector(editingConnector) as Connector;
      connectors = connectors.some((connector) => connector.id === item.id) ? connectors.map((connector) => connector.id === item.id ? item : connector) : [...connectors, item];
      editingConnector = undefined;
    } catch (cause) {
      error = messageFrom(cause);
    }
  }

  async function deleteConnector(): Promise<void> {
    if (!editingConnector?.id) {
      return;
    }
    error = '';
    try {
      await client.DeleteConnector(editingConnector.id);
      connectors = connectors.filter((connector) => connector.id !== editingConnector?.id);
      editingConnector = undefined;
    } catch (cause) {
      error = messageFrom(cause);
    }
  }

  async function newSession(targetProject = projectId): Promise<void> {
    if (!targetProject) {
      return;
    }
    error = '';
    try {
      const item = await client.NewSession(targetProject, botId) as SessionDetail;
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

  async function requestProjectDelete(project: Project): Promise<void> {
    error = '';
    if (!loadedProjects.includes(project.id)) {
      await loadSessions(project.id);
    }
    if ((sessionsByProject[project.id] ?? []).length > 0) {
      error = 'Delete this directory’s chats first.';
      return;
    }
    pendingProjectDelete = project;
  }

  async function deleteProject(): Promise<void> {
    if (!pendingProjectDelete) {
      return;
    }
    const project = pendingProjectDelete;
    pendingProjectDelete = undefined;
    error = '';
    try {
      await client.DeleteProject(project.id);
      projects = projects.filter((item) => item.id !== project.id);
      expandedProjects = expandedProjects.filter((id) => id !== project.id);
      loadedProjects = loadedProjects.filter((id) => id !== project.id);
      delete sessionsByProject[project.id];
      localStorage.setItem('ax-expanded-projects', JSON.stringify(expandedProjects));
      if (projectId === project.id) {
        projectId = projects[0]?.id ?? '';
        sessionId = '';
        messages = [];
        localStorage.removeItem('ax-project');
      }
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
      await client.DeleteSession(session.id);
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
      const item = await client.OpenSession(id) as SessionDetail;
      projectId = item.project_id;
      botId = item.bot_id;
      sessionId = item.id;
      const restored = restoreTranscript(item.messages ?? []);
      const restoredArtifacts = (item.artifacts ?? []).map((artifact): ArtifactItem => ({ kind: 'artifact', ...artifact, session_id: item.id, source: browser ? artifactPath(item.id, artifact.id) : undefined }));
      restored.push(...restoredArtifacts);
      restoredArtifacts.forEach((artifact) => void prepareArtifact(artifact));
      const cached = transcriptsBySession[id];
      messages = cached && isRunning(id) ? cached : restored;
      transcriptsBySession[id] = messages;
      localStorage.setItem('ax-project', item.project_id);
      sidebarOpen = false;
      await scrollToEnd(true);
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
    await scrollToEnd(true);

    try {
      await client.Send(targetSession, text);
    } catch (cause) {
      transcriptsBySession[targetSession]?.pop();
      runningSessions = runningSessions.filter((id) => id !== targetSession);
      error = messageFrom(cause);
    }
  }

  async function cancel(): Promise<void> {
    try {
      await client.Cancel(sessionId);
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
    if (event.type === 'artifact' && event.id && event.name) {
      const artifact: ArtifactItem = { kind: 'artifact', id: event.id, name: event.name, media_type: event.media_type ?? 'application/octet-stream', size: event.size ?? 0, session_id: targetSession, source: browser ? artifactPath(targetSession, event.id) : undefined };
      target.push(artifact);
      void prepareArtifact(artifact);
      if (targetSession === sessionId) {
        void scrollToEnd();
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

  function artifactPath(session: string, artifact: string): string {
    return `/api/sessions/${encodeURIComponent(session)}/artifacts/${encodeURIComponent(artifact)}`;
  }

  async function prepareArtifact(item: ArtifactItem): Promise<void> {
    if (item.source || !item.media_type.startsWith('image/')) {
      return;
    }
    try {
      item.source = await client.ArtifactSource(item.session_id, item.id);
      if (item.session_id === sessionId) {
        messages = [...messages];
      }
    } catch (cause) {
      error = messageFrom(cause);
    }
  }

  async function downloadArtifact(item: ArtifactItem): Promise<void> {
    try {
      const source = item.source ?? await client.ArtifactSource(item.session_id, item.id);
      const link = document.createElement('a');
      link.href = source;
      link.download = item.name;
      link.click();
    } catch (cause) {
      error = messageFrom(cause);
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
    const clean = DOMPurify.sanitize(marked.parse(text, { async: false }) as string);
    if (!clean.includes('<table')) {
      return clean;
    }
    const document = new DOMParser().parseFromString(clean, 'text/html');
    for (const table of document.querySelectorAll('table')) {
      const rows = [...table.querySelectorAll('tbody tr')];
      const columns = table.querySelectorAll('thead th').length;
      for (let column = 0; column < columns; column++) {
        const cells = rows.map((row) => row.children.item(column)).filter((cell): cell is Element => cell !== null);
        const values = cells.map((cell) => cell.textContent?.trim() ?? '').filter((value) => value && value !== '—' && value !== '-');
        if (values.length > 0 && values.every(isNumericValue)) {
          table.querySelector(`thead th:nth-child(${column + 1})`)?.classList.add('numeric');
          cells.forEach((cell) => cell.classList.add('numeric'));
        }
      }
      table.querySelectorAll('thead th').forEach((cell) => cell.setAttribute('scope', 'col'));
      const wrapper = document.createElement('div');
      wrapper.className = 'table-scroll';
      wrapper.tabIndex = 0;
      wrapper.setAttribute('role', 'region');
      wrapper.setAttribute('aria-label', 'Scrollable table');
      table.replaceWith(wrapper);
      wrapper.append(table);
    }
    return document.body.innerHTML;
  }

  function isNumericValue(value: string): boolean {
    return /^\(?[+−-]?(?:[$€£¥]\s*)?(?:\d{1,3}(?:[,\s]\d{3})*|\d+)(?:\.\d+)?(?:\s*%)?\)?$/.test(value);
  }

  function messageFrom(cause: unknown): string {
    return cause instanceof Error ? cause.message : String(cause);
  }

  function trackScroll(): void {
    if (!transcript) {
      return;
    }
    atBottom = transcript.scrollHeight - transcript.scrollTop - transcript.clientHeight < 80;
  }

  async function scrollToEnd(force = false): Promise<void> {
    if (!force && !atBottom) {
      return;
    }
    await tick();
    transcript?.scrollTo({ top: transcript.scrollHeight });
    atBottom = true;
  }
</script>

{#if stopped}
  <main class="connect-shell">
    <div class="connect-card setup-loading"><div class="mark">AX</div><div><h1>Axi stopped</h1><p>You can close this tab. Run Axi to start it again.</p></div></div>
  </main>
{:else if setupChecking}
  <main class="connect-shell">
    <div class="connect-card setup-loading"><div class="mark">AX</div><p>Starting Axi…</p></div>
  </main>
{:else if setupRequired}
  <main class="connect-shell">
    <form class="connect-card" onsubmit={(event) => { event.preventDefault(); void saveSetup(); }}>
      <button class="theme-toggle connect-theme" type="button" onclick={toggleTheme} aria-label={darkMode ? 'Use light mode' : 'Use dark mode'} title={darkMode ? 'Use light mode' : 'Use dark mode'}>
        {#if darkMode}<Sun aria-hidden="true" />{:else}<Moon aria-hidden="true" />{/if}
      </button>
      <div class="mark">AX</div>
      <div><h1>Welcome to Axi</h1><p>Connect your model provider to start chatting.</p></div>
      <label>API key <span class="optional">Optional for local providers</span><input bind:value={setupAPIKey} type="password" autocomplete="off" placeholder="sk-…" /></label>
      <label>Model<input bind:value={setupModel} placeholder="gpt-4.1-mini" /></label>
      <label>Custom base URL <span class="optional">Optional</span><input bind:value={setupBase} type="url" inputmode="url" placeholder="https://api.openai.com/v1" /></label>
      {#if error}<p class="error" role="alert">{error}</p>{/if}
      <div class="setup-actions">
        {#if setupExisting}<button class="quiet-action" type="button" onclick={() => setupRequired = false}>Cancel</button>{/if}
        <button class="primary" type="submit" disabled={loading || (!setupExisting && !setupAPIKey.trim() && !setupBase.trim())}>{loading ? 'Saving…' : setupExisting ? 'Save provider' : 'Start chatting'}</button>
      </div>
      <p class="setup-note">Your key stays on this machine.</p>
    </form>
  </main>
{:else if !connected}
  <main class="connect-shell">
    <form class="connect-card" onsubmit={(event) => { event.preventDefault(); void connect(); }}>
      <button class="theme-toggle connect-theme" type="button" onclick={toggleTheme} aria-label={darkMode ? 'Use light mode' : 'Use dark mode'} title={darkMode ? 'Use light mode' : 'Use dark mode'}>
        {#if darkMode}<Sun aria-hidden="true" />{:else}<Moon aria-hidden="true" />{/if}
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
      <button class="mobile-menu" onclick={() => sidebarOpen = !sidebarOpen} aria-label="Sessions"><Menu aria-hidden="true" /></button>
      <strong class="active-project">{projects.find((project) => project.id === projectId)?.name ?? 'No project selected'}</strong>
      <button class="theme-toggle" onclick={toggleTheme} aria-label={darkMode ? 'Use light mode' : 'Use dark mode'} title={darkMode ? 'Use light mode' : 'Use dark mode'}>
        {#if darkMode}<Sun aria-hidden="true" />{:else}<Moon aria-hidden="true" />{/if}
      </button>
    </header>

    <div class="workspace">
      <aside class:open={sidebarOpen}>
        <div class="sidebar-header">
          <div class="mark small">AX</div>
          <strong>Axi</strong>
          <button class="theme-toggle" onclick={toggleTheme} aria-label={darkMode ? 'Use light mode' : 'Use dark mode'} title={darkMode ? 'Use light mode' : 'Use dark mode'}>
            {#if darkMode}<Sun aria-hidden="true" />{:else}<Moon aria-hidden="true" />{/if}
          </button>
        </div>
        <div class="bot-selector">
          <div class="sidebar-label"><span>Bots</span><button onclick={() => editBot()} aria-label="New bot" title="New bot"><Plus aria-hidden="true" /></button></div>
          {#each bots as bot}
            <div class="bot-row" class:active={bot.id === botId}>
              <button class="bot-select" onclick={() => selectBot(bot.id)}><BotIcon aria-hidden="true" /><span>{bot.name}</span></button>
              <button class="bot-edit" onclick={() => editBot(bot)} aria-label={`Edit ${bot.name}`} title="Edit bot"><Settings aria-hidden="true" /></button>
            </div>
          {:else}
            <p>No bots configured</p>
          {/each}
        </div>
        <div class="bot-selector connector-selector">
          <div class="sidebar-label"><span>Interfaces</span><button onclick={() => editConnector()} aria-label="New interface" title="New interface"><Plus aria-hidden="true" /></button></div>
          {#each connectors as connector}
            <div class="bot-row">
              <button class="bot-select" onclick={() => editConnector(connector)}><Plug aria-hidden="true" /><span>{connector.name}</span></button>
              <i class:running={connector.status === 'running'} title={connector.error || connector.status}></i>
            </div>
          {:else}
            <p>No interfaces configured</p>
          {/each}
        </div>
        <button class="add-project top" onclick={() => void openPicker()}>+ Add directory</button>
        <nav class="projects">
          {#each projects as project}
            <section class:current={project.id === projectId}>
              <div class="project-row">
                <button class="project-toggle" onclick={() => void toggleProject(project.id)} aria-expanded={expandedProjects.includes(project.id)}>
                  <ChevronRight class={expandedProjects.includes(project.id) ? 'expanded' : ''} aria-hidden="true" />
                  <strong>{project.name}</strong>
                </button>
                <button class="project-remove" onclick={() => void requestProjectDelete(project)} aria-label={`Remove ${project.name}`} title="Remove directory">
                  <FolderMinus aria-hidden="true" />
                </button>
                <button class="project-new" onclick={() => void newSession(project.id)} aria-label={`New chat in ${project.name}`} title="New chat">
                  <MessageSquarePlus aria-hidden="true" />
                </button>
              </div>
              {#if expandedProjects.includes(project.id)}
                <div class="project-sessions">
                  {#each sessionsByProject[project.id] ?? [] as session}
                    <div class="session-row" class:active={session.id === sessionId}>
                      <button class="session-open" onclick={() => void openSession(session.id)}><span>{session.title}</span>{#if isRunning(session.id)}<i class="session-running" aria-label="Running"></i>{/if}</button>
                      <button class="session-delete" onclick={() => requestDelete(project, session)} disabled={isRunning(session.id)} aria-label={`Delete ${session.title}`} title="Delete chat">
                        <Trash2 aria-hidden="true" />
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
        <div class="sidebar-server">
          <span class="server-url">{browser ? 'Axis via Axi Web' : baseUrl}</span>
          {#if browser}
            <button class="quiet server-action" onclick={openSetup} aria-label="Change provider" title="Change provider"><SlidersHorizontal aria-hidden="true" /><span>Model provider</span></button>
            <button class="quiet server-action" onclick={() => void quitAxi()} aria-label="Quit Axi" title="Quit Axi"><Power aria-hidden="true" /><span>Quit Axi</span></button>
          {:else}
            <button class="quiet server-action" onclick={() => { connected = false; messages = []; }} aria-label="Change server" title="Change server"><SlidersHorizontal aria-hidden="true" /><span>Change server</span></button>
          {/if}
        </div>
      </aside>
      {#if sidebarOpen}<button class="scrim" onclick={() => sidebarOpen = false} aria-label="Close sessions"></button>{/if}

      <section class="chat">
        <div class="transcript" bind:this={transcript} onscroll={trackScroll} aria-live="polite">
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
            {:else if item.kind === 'artifact'}
              <div class="artifact-card" class:image={item.media_type.startsWith('image/')}>
                {#if item.media_type.startsWith('image/') && item.source}
                  <img src={item.source} alt={item.name} />
                {:else}
                  <Paperclip aria-hidden="true" />
                {/if}
                <span><strong>{item.name}</strong><small>{item.media_type} · {Math.ceil(item.size / 1024)} KB</small></span>
                <button onclick={() => void downloadArtifact(item)}>Download</button>
              </div>
            {:else}
              <div class="tool-card" class:failed={item.status === 'failed'} class:running={item.status === 'running'} class:expanded={item.expanded}>
                <button class="tool-head" onclick={() => item.expanded = !item.expanded} aria-expanded={item.expanded}>
                  <span class="tool-icon">
                    {#if item.name === 'bash' || item.name === 'bashx'}
                      <Terminal aria-hidden="true" />
                    {:else if item.name === 'read' || item.name === 'write' || item.name === 'edit'}
                      <FileText aria-hidden="true" />
                    {:else}
                      <Wrench aria-hidden="true" />
                    {/if}
                  </span>
                  <strong>{item.name}</strong>
                  <span class="tool-summary">{toolSummary(item)}</span>
                  <span class="tool-status">{item.status === 'running' ? 'Running' : item.status === 'failed' ? 'Failed' : 'Done'}</span>
                  <ChevronRight class={`tool-chevron${item.expanded ? ' expanded' : ''}`} aria-hidden="true" />
                </button>
                {#if item.expanded}
                  <div class="tool-detail">
                    {#if item.arguments}
                      <div class="tool-section"><span>Arguments</span><pre>{prettyArguments(item.arguments)}</pre></div>
                    {/if}
                    {#if item.output}
                      <div class="tool-section">
                        <div class="tool-section-head"><span>Output</span><button onclick={() => void navigator.clipboard.writeText(item.output)}><Copy aria-hidden="true" />Copy</button></div>
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

        {#if !atBottom}
          <button class="jump-latest" onclick={() => void scrollToEnd(true)} aria-label="Jump to latest"><ArrowDown aria-hidden="true" /><span>Latest</span></button>
        {/if}

        <footer>
          {#if error}<p class="error footer-error" role="alert">{error}</p>{/if}
          <div class="composer">
            <textarea bind:this={composer} bind:value={prompt} onkeydown={keydown} rows="1" placeholder="Message AX" aria-label="Message AX"></textarea>
            {#if isRunning(sessionId)}
              <button class="stop" onclick={() => void cancel()} aria-label="Stop">■</button>
            {:else}
              <button class="send" onclick={() => void send()} disabled={!prompt.trim()} aria-label="Send"><ArrowUp aria-hidden="true" /></button>
            {/if}
          </div>
          <p class="hint">Enter to send · Shift+Enter for a new line</p>
        </footer>
      </section>
    </div>
  </main>

  {#if editingConnector}
    <div class="modal-backdrop" role="presentation">
      <div class="bot-dialog connector-dialog" role="dialog" aria-modal="true" aria-labelledby="connector-title">
        <div class="picker-head">
          <div><h2 id="connector-title">{editingConnector.id ? 'Edit interface' : 'New interface'}</h2><p>Axis supervises enabled interfaces.</p></div>
          <button class="close" onclick={() => editingConnector = undefined} aria-label="Close"><X aria-hidden="true" /></button>
        </div>
        <div class="bot-form">
          <label>Name<input bind:value={editingConnector.name} maxlength="80" /></label>
          <label>Type<select bind:value={editingConnector.type}><option value="slack">Slack</option></select></label>
          <label>Bot<select bind:value={editingConnector.bot_id}>{#each bots as bot}<option value={bot.id}>{bot.name}</option>{/each}</select></label>
          <label>Project<select bind:value={editingConnector.project_id}>{#each projects as project}<option value={project.id}>{project.name}</option>{/each}</select></label>
          <label class="connector-enabled"><input bind:checked={editingConnector.enabled} type="checkbox" /> Enabled</label>
          {#if editingConnector.id}<p class="connector-secret">Credentials: ~/.config/axis/connectors/{editingConnector.id}.env</p>{/if}
          {#if editingConnector.error}<p class="error">{editingConnector.error}</p>{/if}
          {#if error}<p class="error" role="alert">{error}</p>{/if}
          <div class="bot-actions">
            {#if editingConnector.id}<button class="delete-action" onclick={() => void deleteConnector()}>Delete interface</button>{/if}
            <span></span>
            <button class="quiet-action" onclick={() => editingConnector = undefined}>Cancel</button>
            <button class="primary-action" onclick={() => void saveConnector()} disabled={!editingConnector.name.trim() || !editingConnector.bot_id || !editingConnector.project_id}>Save interface</button>
          </div>
        </div>
      </div>
    </div>
  {/if}

  {#if editingBot}
    <div class="modal-backdrop" role="presentation">
      <div class="bot-dialog" role="dialog" aria-modal="true" aria-labelledby="bot-title">
        <div class="picker-head">
          <div><h2 id="bot-title">{editingBot.id ? 'Edit bot' : 'New bot'}</h2><p>Changes apply to all of this bot’s chats.</p></div>
          <button class="close" onclick={() => editingBot = undefined} aria-label="Close"><X aria-hidden="true" /></button>
        </div>
        <div class="bot-form">
          <label>Name<input bind:value={editingBot.name} maxlength="80" /></label>
          <label>System prompt<textarea bind:value={editingBot.prompt} rows="8"></textarea></label>
          <label>Workspace root<span class="path-field"><input bind:value={editingBot.workspace_root} readonly /><button onclick={() => void openPicker('workspace')}>Choose</button></span></label>
          <label>Skill root<span class="path-field"><input bind:value={editingBot.skill_root} readonly /><button onclick={() => void openPicker('skills')}>Choose</button></span></label>
          <label>Tools<input bind:value={botTools} placeholder="fsx bashx skillx" /></label>
          <label>Model<input bind:value={editingBot.model} placeholder="Use Axis default" /></label>
          {#if error}<p class="error" role="alert">{error}</p>{/if}
          <div class="bot-actions">
            {#if editingBot.id}<button class="delete-action" onclick={() => void deleteBot()}>Delete bot</button>{/if}
            <span></span>
            <button class="quiet-action" onclick={() => editingBot = undefined}>Cancel</button>
            <button class="primary-action" onclick={() => void saveBot()} disabled={!editingBot.name.trim() || !editingBot.prompt.trim() || !editingBot.workspace_root.trim()}>Save bot</button>
          </div>
        </div>
      </div>
    </div>
  {/if}

  {#if pendingProjectDelete}
    <div class="modal-backdrop delete-backdrop" role="presentation">
      <div class="delete-dialog" role="alertdialog" aria-modal="true" aria-labelledby="remove-project-title" aria-describedby="remove-project-description">
        <div class="delete-icon">
          <FolderMinus aria-hidden="true" />
        </div>
        <div>
          <h2 id="remove-project-title">Remove directory?</h2>
          <p id="remove-project-description"><strong>{pendingProjectDelete.name}</strong> will be removed from Axi. Its files will not be changed.</p>
        </div>
        <div class="delete-actions">
          <button class="quiet-action" onclick={() => pendingProjectDelete = undefined}>Cancel</button>
          <button class="delete-action" onclick={() => void deleteProject()}>Remove directory</button>
        </div>
      </div>
    </div>
  {/if}

  {#if pendingDelete}
    <div class="modal-backdrop delete-backdrop" role="presentation">
      <div class="delete-dialog" role="alertdialog" aria-modal="true" aria-labelledby="delete-title" aria-describedby="delete-description">
        <div class="delete-icon">
          <Trash2 aria-hidden="true" />
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
          <div><h2 id="picker-title">{pickerTarget === 'project' ? 'Add directory' : pickerTarget === 'workspace' ? 'Choose workspace' : 'Choose skill root'}</h2><p>{directoryPath || 'Choose a location'}</p></div>
          <button class="close" onclick={() => pickerOpen = false} aria-label="Close"><X aria-hidden="true" /></button>
        </div>

        {#if directoryPath}
          <input class="folder-filter" bind:value={directoryFilter} type="search" placeholder="Filter folders" aria-label="Filter folders" />
        {/if}

        <div class="directory-list">
          {#if !directoryPath}
            {#each directoryRoots as root}
              <button onclick={() => void browse(root.path)}><span class="folder-icon"><Folder aria-hidden="true" /></span><span><strong>{root.name}</strong><small>{root.path}</small></span><b>›</b></button>
            {/each}
          {:else}
            {#if directoryParent}<button onclick={() => void browse(directoryParent)}><span class="folder-icon"><FolderUp aria-hidden="true" /></span><span><strong>Parent directory</strong></span></button>{/if}
            {#each visibleDirectories() as directory}
              <button onclick={() => void browse(directory.path)}>
                <span class="folder-icon"><Folder aria-hidden="true" /></span>
                <span><strong>{directory.name}</strong><small>{directory.registered ? 'Already added' : directory.kind ?? ''}</small></span>
                <b>›</b>
              </button>
            {/each}
          {/if}
          {#if pickerLoading}<p class="picker-status">Loading…</p>{/if}
        </div>

        {#if directoryPath}
          <div class="picker-form">
            {#if pickerTarget === 'project'}<label>Project name<input bind:value={projectName} maxlength="80" /></label>{/if}
            {#if error}<p class="error" role="alert">{error}</p>{/if}
            <div class="picker-actions">
              <button class="quiet-action" onclick={() => pickerOpen = false}>Cancel</button>
              {#if pickerTarget === 'project'}
                <button class="primary-action" onclick={() => void addProject()} disabled={pickerLoading || !projectName.trim()}>Add this directory</button>
              {:else}
                <button class="primary-action" onclick={chooseBotDirectory}>Use this directory</button>
              {/if}
            </div>
          </div>
        {/if}
      </div>
    </div>
  {/if}
{/if}
