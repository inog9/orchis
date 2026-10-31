export type User = {
  id: string;
  email: string;
  display_name: string;
  role: string;
  permissions: string[];
};

export type AssetAddress = { kind: string; value: string; is_primary: boolean };

export type TagRef = { id: string; name: string; color: string };
export type GroupRef = { id: string; name: string; is_system: boolean };

export type Asset = {
 organization_id?: string | null;
  id: string;
  category: string;
  name: string;
  description: string | null;
  platform: string | null;
  arch: string | null;
  criticality: 'low' | 'medium' | 'high' | 'critical';
  is_internet_facing: boolean;
  testing_authorized: boolean;
  source: string;
  agent_state?: 'online' | 'offline' | 'none';
  attributes: Record<string, unknown>;
  addresses: AssetAddress[];
  tags: TagRef[];
  groups?: GroupRef[];
  created_at: string;
  updated_at: string;
  version: number;
};

export type AssetPage = { items: Asset[]; next_cursor: string | null };

export type NewAsset = {
  category: string;
  name: string;
  platform?: string | null;
  arch?: string | null;
  criticality: string;
  is_internet_facing: boolean;
  testing_authorized: boolean;
  addresses: { kind: string; value: string }[];
  tag_ids?: string[];
};

export type AssetPatch = {
  version: number;
  name?: string;
  platform?: string;
  arch?: string;
  clear_platform?: boolean;
  clear_arch?: boolean;
  criticality?: string;
  is_internet_facing?: boolean;
  testing_authorized?: boolean;
  addresses?: { kind: string; value: string }[];
  tag_ids?: string[];
};

export type Filter = { field: string; operator: string; values: string[] };
export type Rule = { mode: 'and' | 'or'; filters: Filter[] };

export type Group = {
  id: string;
  name: string;
  description: string | null;
  kind: string;
  rule: Rule;
  is_system: boolean;
  member_count: number;
  created_at: string;
  updated_at: string;
  version: number;
};
export type GroupMember = { id: string; name: string; category: string; platform: string | null; arch: string | null; criticality: string };
export type GroupDetail = Group & { members: GroupMember[]; total_assets: number; members_capped: boolean };
export type RulePreview = { count: number; total: number; sample: GroupMember[] };

export type CsvColumn = { name: string; sample: string[] };
export type CsvIssue = { row: number; message: string };
export type ImportPreview = {
  columns: CsvColumn[];
  suggested_mapping: Record<string, string>;
  rows: number;
  validated: boolean;
  valid: number;
  invalid: CsvIssue[];
  duplicates: CsvIssue[];
};
export type ImportResult = { created: number; skipped_invalid: number; skipped_duplicates: number; tags_created: number };

export const can = (u: User | null | undefined, perm: string) => !!u?.permissions.includes(perm);

// ---------- Fase 3: atomic test, eksekusi, finding ----------
export type Contract = {
 version?: number; category?: string; input_schema?: import('../components/SchemaFields').ParameterSchema;
  key: string;
  name: string;
  description: string;
  kind: string;
  output_types: string[];
  timeout_seconds: number;
  integration: string;
  integration_status: 'online' | 'offline' | 'disabled';
  last_heartbeat_at: string | null;
  needs_agent: boolean;
};

export type ExecutionSummary = {
  id: string;
  state: 'ready' | 'running' | 'ended';
  outcome: 'success' | 'error' | 'canceled' | 'timeout' | 'skipped' | null;
  error: string | null;
  created_at: string;
  started_at: string | null;
  ended_at: string | null;
  finding_count: number;
};

export type NmapParams = { profile: string; ports?: string; service_detection: boolean };
export type NucleiParams = { severity: string[]; tags?: string[] };

export type AtomicTest = {
  id: string;
  name: string;
  contract_key: string;
  contract_name: string;
  params: NmapParams | NucleiParams | Record<string, unknown>;
  target_asset_id: string | null;
  target_asset_name: string | null;
  created_at: string;
  updated_at: string;
  version: number;
  last_execution: ExecutionSummary | null;
  executions?: ExecutionSummary[];
};

export type NewAtomicTest = { name: string; contract_key: string; params: NmapParams | NucleiParams | Record<string, unknown>; target_asset_id: string | null };

export type Finding = {
  id: string;
  asset_id: string | null;
  asset_name: string | null;
  type: string;
  value: string;
  title: string;
  severity: 'info' | 'low' | 'medium' | 'high' | 'critical';
  details: Record<string, unknown>;
  first_seen_at: string;
  last_seen_at: string;
  occurrence_count: number;
};

export type FindingPage = { items: Finding[]; next_cursor: string | null; search: string };

export type ExecutionDetail = ExecutionSummary & {
  atomic_test_id: string | null;
  contract_key: string;
  target_asset_id: string;
  target_address: string;
  params: Record<string, unknown>;
  exit_code: number | null;
  raw_output_key: string | null;
  logs: { at: string; level: string; message: string }[];
  findings: Finding[];
};

// ---------- Home ----------
export type HomeSummary = {
 organizations?: { id:string;name:string;assets:number;authorized:number;risk:string;comparison:string;latest_assessment:string|null;latest_status:string }[] | null;
  assets: { total: number; authorized: number; internet_facing: number } | null;
  findings: { total: number; vulnerabilities: number; open_ports: number; vulnerable_assets: number; by_severity: Record<string, number>; new_7d: number } | null;
  atomic: { tests: number; active: number; runs_7d: number; failed_7d: number } | null;
  chains: { scenarios: number; simulations: number; running: number } | null;
  latest_simulations: { id: string; code: string; name: string; status: Simulation['status']; started_at: string | null; created_at: string }[];
  latest_runs: { execution_id: string; test_id: string; test_name: string; contract_name: string; target: string | null; state: ExecutionSummary['state']; outcome: ExecutionSummary['outcome']; created_at: string; finding_count: number }[];
  most_exposed: { asset_id: string; name: string; critical: number; high: number; medium: number; low: number; open_ports: number }[];
  injectors: { key: string; name: string; status: 'online' | 'offline' | 'disabled' }[];
  generated_at: string;
};

// ---------- Fase 5-6: scenario, chain, simulasi ----------
export type Trigger = { type: string; values?: string[]; min_severity?: string };
export type ChainStep = {
  key: string;
  title: string;
  description: string;
  contract_key: string;
  params: Record<string, unknown>;
  delay_seconds: number;
  trigger_mode: 'after_parents' | 'on_finding';
  trigger: Trigger | null;
  is_enabled: boolean;
};
export type ChainEdge = { parent: string; child: string; condition?: Record<string, unknown>; branch?: 'match' | 'else' | 'failure'; mapper?: { source: 'open_ports' | 'web_urls'; destination: 'ports' | 'target' } };
export type ScenarioTarget = { kind: 'asset' | 'group'; id: string; name: string; authorized?: boolean };
export type Scenario = {
  id: string;
  name: string;
  description: string | null;
  severity: 'low' | 'medium' | 'high' | 'critical';
  category: string | null;
  status: 'draft' | 'ready';
  step_count: number;
  edge_count: number;
  target_count: number;
  simulations: number;
  created_at: string;
  updated_at: string;
  version: number;
};
export type ScenarioDetail = Scenario & { targets: ScenarioTarget[]; steps: ChainStep[]; edges: ChainEdge[]; problems: string[] };

export type StepStatus = 'waiting' | 'queued' | 'running' | 'done' | 'failed' | 'skipped' | 'blocked' | 'canceled';
export type Simulation = {
  id: string;
  number: number;
  code: string;
  name: string;
  status: 'draft' | 'scheduled' | 'running' | 'paused' | 'finished' | 'canceled' | 'failed';
  scenario_id: string | null;
  scenario_name: string | null;
  safe_mode: boolean;
  timeout_seconds: number;
  cancel_reason: string | null;
  started_at: string | null;
  ended_at: string | null;
  created_at: string;
  target_count: number;
  step_count: number;
  steps_done: number;
  finding_count: number;
};
export type SimStep = {
  key: string;
  title: string;
  contract_key: string;
  contract_name: string;
  status: StepStatus;
  trigger_mode: string;
  trigger: Trigger | null;
  delay_seconds: number;
  is_enabled: boolean;
  executions: Record<string, number>;
  finding_count: number;
};
export type SimulationDetail = Simulation & { steps: SimStep[]; edges: ChainEdge[]; targets: { id: string; name: string }[] };
export type PathNode = { id: string; kind: 'step' | 'asset' | 'finding'; label: string; sub?: string; status?: string; severity?: string; column: number };
export type PathEdge = { from: string; to: string; kind: 'targets' | 'found' | 'triggers' | 'then' };
export type AttackPath = { nodes: PathNode[]; edges: PathEdge[]; truncated: boolean };

// ---------- Fase 7: akun, pengaturan, orang, MITRE, notifikasi ----------
export type UserRow = { id: string; email: string; display_name: string; role: string; role_name: string; status: 'active' | 'disabled'; last_login_at: string | null; created_at: string; version: number; has_password: boolean };
export type RoleRow = { key: string; name: string; description: string | null; permissions: string[]; is_system: boolean; users: number };
export type ResetLink = { token: string; expires_at: string };
export type CreatedUser = { user: UserRow; reset_link: ResetLink };
export type ApiToken = { id: string; name: string; prefix: string; created_at: string; last_used_at: string | null; expires_at: string | null };
export type NewApiToken = ApiToken & { secret: string };

export type IntegrationRow = {
  id: string; kind: 'injector' | 'collector' | 'executor'; key: string; name: string; description: string | null; is_builtin: boolean; enabled: boolean;
  status: 'online' | 'offline' | 'disabled'; software_version: string | null; last_heartbeat_at: string | null; has_token: boolean;
  logs: { at: string; level: string; message: string }[];
};
export type PlatformSettings = { platform_name: string; version: string; components: { name: string; status: 'up' | 'down' | 'not_configured'; detail?: string }[] };
export type TagRow = { id: string; name: string; color: string; assets: number };

export type PersonRow = { id: string; first_name: string; last_name: string; email: string | null; organization: string | null; teams: { id: string; name: string }[]; created_at: string; version: number };
export type TeamRow = { id: string; name: string; description: string | null; kind: 'internal' | 'external'; member_count: number; created_at: string; version: number };
export type TeamDetail = TeamRow & { members: { id: string; name: string; email: string | null }[] };

export type MitreCell = { id: string; name: string; mapped: boolean; exercised: boolean; simulated?: boolean; platforms?: string[]; runs: number; last_run_at: string | null; subs?: MitreCell[] };
export type MitreMatrix = {
  tactics: { id: string; name: string; shortname: string; techniques: MitreCell[] }[];
  total: number; mapped: number; exercised: number; simulated?: number; platforms?: string[]; imported: boolean;
  last_run: { id: string; status: 'running' | 'succeeded' | 'failed'; source_version: string | null; techniques: number | null; tactics: number | null; subtechniques?: number | null; deprecated?: number | null; source_sha256?: string | null; error: string | null; started_at: string; finished_at: string | null } | null;
};
export type MitreMapping = { action: string; reason: string; basis: 'direct' | 'estimate'; via?: string };
export type MitreDetail = { id: string; name: string; description: string; platforms: string[]; tactics: string[]; url: string; actions: string[]; runs: number; last_run_at: string | null; subs: MitreCell[]; mappings?: MitreMapping[]; mitigations?: { id: string; name: string; description: string }[]; parent?: string | null; deprecated?: boolean; simulated?: boolean; simulated_findings?: number };

export type AlertRow = { id: string; kind: string; severity: 'info' | 'warning' | 'critical'; title: string; body: string | null; resource_type: string | null; resource_id: string | null; created_at: string; read: boolean };
export type AlertList = { items: AlertRow[]; unread: number };

// ---------- Fase 4: agent ----------
export type AgentRow = {
 organization_id?: string | null; instance_name?: string | null;
  id: string; asset_id: string; asset_name: string; hostname: string | null; os: 'windows' | 'linux' | 'macos'; arch: 'x86_64' | 'arm64'; mode: 'service' | 'user';
  version: string; status: 'online' | 'offline' | 'revoked'; last_heartbeat_at: string | null; enrolled_at: string; update_available: boolean;
};
export type EnrollmentToken = { id: string; token: string; os: AgentRow['os']; arch: AgentRow['arch']; mode: AgentRow['mode']; expires_at: string; version: string; sha256: string; file_name: string };
export type EnrollmentStatus = { used: boolean; expired: boolean; agent_id: string | null; agent: AgentRow | null };
