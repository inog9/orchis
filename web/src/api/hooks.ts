import { useEndpointStream } from './endpointStream';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, ApiError } from './client';
import type { EndpointAssetData, EndpointCatalog, EndpointConsent, OrgOsquerySetting, AgentRow, EnrollmentStatus, EnrollmentToken, AlertList, ApiToken, CreatedUser, IntegrationRow, MitreDetail, MitreMatrix, NewApiToken, PersonRow, PlatformSettings, ResetLink, RoleRow, TagRow, TeamDetail, TeamRow, UserRow, Asset, AssetPage, AssetPatch, AtomicTest, Contract, AttackPath, ChainEdge, ChainStep, ExecutionDetail, FindingPage, HomeSummary, Scenario, ScenarioDetail, Simulation, SimulationDetail, Group, GroupDetail, ImportPreview, ImportResult, NewAsset, NewAtomicTest, Rule, RulePreview, TagRef, User } from './types';

export function useMe() {
  return useQuery({
    queryKey: ['me'],
    queryFn: async () => {
      try {
        return await api.get<User>('/api/v1/auth/me');
      } catch (e) {
        if (e instanceof ApiError && e.status === 401) return null;
        throw e;
      }
    },
    retry: false,
    staleTime: 60_000,
  });
}

export function useLogin() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { email: string; password: string; remember_me: boolean }) => api.post<User>('/api/v1/auth/login', v),
    onSuccess: (u) => qc.setQueryData(['me'], u),
  });
}

export function useLogout() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.post<void>('/api/v1/auth/logout'),
    onSuccess: () => {
      qc.setQueryData(['me'], null);
      qc.removeQueries({ predicate: (q) => q.queryKey[0] !== 'me' });
    },
  });
}

export function useAssets(q: string, category: string, organization = '', enabled=true) {
  return useInfiniteQuery({
    queryKey: ['assets', q, category, organization], enabled,
    initialPageParam: '',
    queryFn: ({ pageParam }) => {
      const p = new URLSearchParams({ limit: '25' });
      if (q) p.set('q', q);
      if (category) p.set('category', category);
      if (organization) p.set('org', organization);
      if (pageParam) p.set('cursor', pageParam);
      return api.get<AssetPage>(`/api/v1/assets?${p}`);
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    retry: false,
  });
}

export function useCreateAsset() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: NewAsset) => api.post<Asset>('/api/v1/assets', v),
    onSuccess: () => Promise.all(['assets','organization-assets','organization-readiness','home'].map(key=>qc.invalidateQueries({queryKey:[key]}))),
  });
}

// ---------- aset: detail, ubah, hapus ----------
export function useAsset(id: string) {
  return useQuery({ queryKey: ['asset', id], queryFn: () => api.get<Asset>(`/api/v1/assets/${id}`), retry: false });
}

// Alamat/izin baru harus segera terlihat pada pratinjau organisasi, tanpa menunggu polling.
function useInvalidateAssets() {
  const qc = useQueryClient();
  return () => Promise.all(['assets','asset','groups','group','tags','organization-assets','organization-readiness','home'].map(key=>qc.invalidateQueries({queryKey:[key]})));
}

export function usePatchAsset(id: string) {
  const inv = useInvalidateAssets();
  return useMutation({ mutationFn: (v: AssetPatch) => api.patch<Asset>(`/api/v1/assets/${id}`, v), onSuccess: inv });
}

export function useDeleteAsset(id: string) {
  const inv = useInvalidateAssets();
  return useMutation({ mutationFn: () => api.del(`/api/v1/assets/${id}`), onSuccess: inv });
}

// ---------- tag ----------
export function useTags() {
  return useQuery({ queryKey: ['tags'], queryFn: () => api.get<{ items: TagRef[] }>('/api/v1/tags').then((r) => r.items), staleTime: 30_000 });
}

export function useCreateTag() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { name: string; color?: string }) => api.post<TagRef>('/api/v1/tags', v),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tags'] }),
  });
}

// ---------- grup ----------
export function useGroups() {
  return useQuery({ queryKey: ['groups'], queryFn: () => api.get<{ items: Group[] }>('/api/v1/asset-groups').then((r) => r.items), retry: false });
}

export function useGroup(id: string) {
  return useQuery({ queryKey: ['group', id], queryFn: () => api.get<GroupDetail>(`/api/v1/asset-groups/${id}`), retry: false });
}

export function useRulePreview(rule: Rule, enabled: boolean) {
  const key = JSON.stringify(rule);
  return useQuery({
    queryKey: ['rule-preview', key],
    queryFn: () => api.post<RulePreview>('/api/v1/asset-groups/preview', { rule }),
    enabled,
    retry: false,
    staleTime: 5_000,
    placeholderData: (prev) => prev,
  });
}

export function useSaveGroup(id?: string) {
  const inv = useInvalidateAssets();
  return useMutation({
    mutationFn: (v: { name: string; description: string | null; rule: Rule; version?: number }) =>
      id ? api.put<GroupDetail>(`/api/v1/asset-groups/${id}`, v) : api.post<GroupDetail>('/api/v1/asset-groups', v),
    onSuccess: inv,
  });
}

export function useDeleteGroup(id: string) {
  const inv = useInvalidateAssets();
  return useMutation({ mutationFn: () => api.del(`/api/v1/asset-groups/${id}`), onSuccess: inv });
}

// ---------- impor CSV ----------
export function useImportPreview() {
  return useMutation({ mutationFn: (v: { csv: string; mapping?: Record<string, string> }) => api.post<ImportPreview>('/api/v1/assets/import/preview', v) });
}

export function useImportAssets() {
  const inv = useInvalidateAssets();
  return useMutation({
    mutationFn: (v: { csv: string; mapping: Record<string, string>; skip_invalid: boolean }) => api.post<ImportResult>('/api/v1/assets/import', v),
    onSuccess: inv,
  });
}

// ---------- Fase 3 ----------
export function useContracts() {
  return useQuery({
    queryKey: ['contracts'],
    queryFn: async () => (await api.get<{ items: Contract[] }>('/api/v1/contracts')).items,
    refetchInterval: 15_000,
    retry: false,
  });
}

export function useAtomicTests() {
  return useQuery({
    queryKey: ['atomic-tests'],
    queryFn: async () => (await api.get<{ items: AtomicTest[] }>('/api/v1/atomic-tests')).items,
    // Selama ada yang berjalan, segarkan daftar agar status terbaru tampil tanpa muat ulang.
    refetchInterval: (q) => (q.state.data?.some((t) => t.last_execution && t.last_execution.state !== 'ended') ? 2000 : false),
    retry: false,
  });
}

export function useAtomicTest(id: string) {
  return useQuery({
    queryKey: ['atomic-test', id],
    queryFn: () => api.get<AtomicTest>(`/api/v1/atomic-tests/${id}`),
    refetchInterval: (q) => (q.state.data?.last_execution && q.state.data.last_execution.state !== 'ended' ? 2000 : false),
    retry: false,
  });
}

export function useExecution(id: string | undefined) {
  return useQuery({
    queryKey: ['execution', id],
    enabled: !!id,
    queryFn: () => api.get<ExecutionDetail>(`/api/v1/executions/${id}`),
    refetchInterval: (q) => (q.state.data && q.state.data.state !== 'ended' ? 1500 : false),
    retry: false,
  });
}

function useInvalidateRuns() {
  const qc = useQueryClient();
  return () => Promise.all([qc.invalidateQueries({ queryKey: ['atomic-tests'] }), qc.invalidateQueries({ queryKey: ['atomic-test'] }), qc.invalidateQueries({ queryKey: ['execution'] }), qc.invalidateQueries({ queryKey: ['findings'] })]);
}

export function useCreateAtomicTest() {
  const inv = useInvalidateRuns();
  return useMutation({ mutationFn: (v: NewAtomicTest) => api.post<AtomicTest>('/api/v1/atomic-tests', v), onSuccess: inv });
}

export function useRunAtomicTest(id: string) {
  const inv = useInvalidateRuns();
  return useMutation({ mutationFn: () => api.post<ExecutionDetail>(`/api/v1/atomic-tests/${id}/run`), onSuccess: inv });
}

export function useCancelExecution() {
  const inv = useInvalidateRuns();
  return useMutation({ mutationFn: (id: string) => api.post<ExecutionDetail>(`/api/v1/executions/${id}/cancel`), onSuccess: inv });
}

export function useDeleteAtomicTest(id: string) {
  const inv = useInvalidateRuns();
  return useMutation({ mutationFn: () => api.del(`/api/v1/atomic-tests/${id}`), onSuccess: inv });
}

export function useFindings(q: string, type: string, severity: string, assetID = '', organization = '', triage = '') {
  return useInfiniteQuery({
    queryKey: ['findings', q, type, severity, assetID, organization, triage],
    initialPageParam: '',
    queryFn: ({ pageParam }) => {
      const p = new URLSearchParams({ limit: '25' });
      if (q) p.set('q', q);
      if (type) p.set('type', type);
      if (severity) p.set('severity', severity);
      if (assetID) p.set('asset_id', assetID);
      if (organization) p.set('org',organization);
      if (triage) p.set('triage', triage);
      if (pageParam) p.set('cursor', pageParam);
      return api.get<FindingPage>(`/api/v1/findings?${p}`);
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    retry: false,
  });
}

// Aset yang boleh dijadikan sasaran: hanya yang sudah diotorisasi. Batas 100 aset pertama (lihat catatan di dokumen).
export function useTargetAssets() {
  return useQuery({
    queryKey: ['assets', 'targets'],
    queryFn: async () => (await api.get<AssetPage>('/api/v1/assets?limit=100')).items.filter((a) => a.testing_authorized),
    retry: false,
  });
}

export function useHome(enabled = true) {
  return useQuery({
    enabled,
    queryKey: ['home'],
    queryFn: () => api.get<HomeSummary>('/api/v1/home'),
    // Selama ada run aktif, angka berubah; selain itu cukup sesekali.
    refetchInterval: (q) => (q.state.data?.atomic?.active ? 3000 : 30_000),
    retry: false,
  });
}

// ---------- Fase 5-6 ----------
export function useScenarios() {
  return useQuery({ queryKey: ['scenarios'], queryFn: async () => (await api.get<{ items: Scenario[] }>('/api/v1/scenarios')).items, retry: false });
}

export function useScenario(id: string) {
  return useQuery({ queryKey: ['scenario', id], queryFn: () => api.get<ScenarioDetail>(`/api/v1/scenarios/${id}`), retry: false });
}

function useInvalidateScenarios() {
  const qc = useQueryClient();
  return () => Promise.all([qc.invalidateQueries({ queryKey: ['scenarios'] }), qc.invalidateQueries({ queryKey: ['scenario'] }), qc.invalidateQueries({ queryKey: ['home'] })]);
}

export function useCreateScenario() {
  const inv = useInvalidateScenarios();
  return useMutation({
    mutationFn: (v: { name: string; description?: string | null; severity: string; category?: string | null }) => api.post<ScenarioDetail>('/api/v1/scenarios', v),
    onSuccess: inv,
  });
}

export function useDeleteScenario(id: string) {
  const inv = useInvalidateScenarios();
  return useMutation({ mutationFn: () => api.del(`/api/v1/scenarios/${id}`), onSuccess: inv });
}

export function useSetTargets(id: string) {
  const inv = useInvalidateScenarios();
  return useMutation({
    mutationFn: (v: { version: number; asset_ids: string[]; group_ids: string[] }) => api.put<ScenarioDetail>(`/api/v1/scenarios/${id}/targets`, v),
    onSuccess: inv,
  });
}

export function useSaveGraph(id: string) {
  const inv = useInvalidateScenarios();
  return useMutation({
    mutationFn: (v: { version: number; steps: ChainStep[]; edges: ChainEdge[] }) => api.put<ScenarioDetail>(`/api/v1/scenarios/${id}/graph`, v),
    onSuccess: inv,
  });
}

export function useLaunch(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.post<SimulationDetail>(`/api/v1/scenarios/${id}/launch`, { name }),
    onSuccess: () => Promise.all([qc.invalidateQueries({ queryKey: ['simulations'] }), qc.invalidateQueries({ queryKey: ['scenarios'] }), qc.invalidateQueries({ queryKey: ['home'] })]),
  });
}

export function useSimulations(status: string, enabled = true) {
  return useQuery({
    enabled, queryKey: ['simulations', status],
    queryFn: async () => (await api.get<{ items: Simulation[] }>(`/api/v1/simulations${status ? `?status=${status}` : ''}`)).items,
    refetchInterval: (q) => (q.state.data?.some((s) => s.status === 'running') ? 3000 : false),
    retry: false,
  });
}

export function useSimulation(id: string) {
  return useQuery({
    queryKey: ['simulation', id],
    queryFn: () => api.get<SimulationDetail>(`/api/v1/simulations/${id}`),
    refetchInterval: (q) => (q.state.data && ['running', 'paused'].includes(q.state.data.status) ? 2000 : false),
    retry: false,
  });
}

export function useAttackPath(id: string, status: string | undefined,enabled=true) {
  return useQuery({
    queryKey: ['attack-path', id, status], enabled,
    queryFn: () => api.get<AttackPath>(`/api/v1/simulations/${id}/attack-path`),
    refetchInterval: status === 'running' ? 3000 : false,
    retry: false,
  });
}

export function useSimulationAction(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ action, reason }: { action: 'pause' | 'resume' | 'cancel'; reason?: string }) => api.post<SimulationDetail>(`/api/v1/simulations/${id}/${action}`, action === 'cancel' ? { reason: reason ?? '' } : undefined),
    onSuccess: () => Promise.all([qc.invalidateQueries({ queryKey: ['simulation'] }), qc.invalidateQueries({ queryKey: ['simulations'] }), qc.invalidateQueries({ queryKey: ['attack-path'] }), qc.invalidateQueries({ queryKey: ['home'] })]),
  });
}

export function useSimulationFindings(simulationId: string) {
  return useInfiniteQuery({
    queryKey: ['findings', 'simulation', simulationId],
    initialPageParam: '',
    queryFn: ({ pageParam }) => {
      const p = new URLSearchParams({ limit: '25', simulation_id: simulationId });
      if (pageParam) p.set('cursor', pageParam);
      return api.get<FindingPage>(`/api/v1/findings?${p}`);
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    retry: false,
  });
}

export function useAllAssets() {
  return useQuery({ queryKey: ['assets', 'all100'], queryFn: async () => (await api.get<AssetPage>('/api/v1/assets?limit=100')).items, retry: false });
}

// ---------- Fase 7 ----------
function inv(qc: ReturnType<typeof useQueryClient>, ...keys: string[]) {
  return Promise.all(keys.map((k) => qc.invalidateQueries({ queryKey: [k] })));
}

export function useUsers() {
  return useQuery({ queryKey: ['users'], queryFn: async () => (await api.get<{ items: UserRow[] }>('/api/v1/users')).items, retry: false });
}
export function useRoles() {
  return useQuery({ queryKey: ['roles'], queryFn: async () => (await api.get<{ items: RoleRow[] }>('/api/v1/roles')).items, retry: false });
}
export function useCreateUser() {
  const qc = useQueryClient();
  return useMutation({ mutationFn: (v: { email: string; display_name: string; role: string }) => api.post<CreatedUser>('/api/v1/users', v), onSuccess: () => inv(qc, 'users', 'roles') });
}
export function usePatchUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...v }: { id: string; version: number; display_name?: string; role?: string; status?: string; accept_role_permissions?: boolean }) => api.patch<UserRow>(`/api/v1/users/${id}`, v),
    onSuccess: () => inv(qc, 'users', 'roles'),
  });
}
export function useResetLink() {
  return useMutation({ mutationFn: (id: string) => api.post<ResetLink>(`/api/v1/users/${id}/reset-link`) });
}
export function useUpdateProfile() {
  const qc = useQueryClient();
  return useMutation({ mutationFn: (display_name: string) => api.patch<void>('/api/v1/me', { display_name }), onSuccess: () => inv(qc, 'me', 'users') });
}
export function useChangePassword() {
  return useMutation({ mutationFn: (v: { current_password: string; new_password: string }) => api.post<void>('/api/v1/me/password', v) });
}
export function useMyTokens() {
  return useQuery({ queryKey: ['my-tokens'], queryFn: async () => (await api.get<{ items: ApiToken[] }>('/api/v1/me/tokens')).items, retry: false });
}
export function useCreateToken() {
  const qc = useQueryClient();
  return useMutation({ mutationFn: (v: { name: string; expires_in_days: number }) => api.post<NewApiToken>('/api/v1/me/tokens', v), onSuccess: () => inv(qc, 'my-tokens') });
}
export function useRevokeToken() {
  const qc = useQueryClient();
  return useMutation({ mutationFn: (id: string) => api.del(`/api/v1/me/tokens/${id}`), onSuccess: () => inv(qc, 'my-tokens') });
}
export function useForgot() {
  return useMutation({ mutationFn: (email: string) => api.post<void>('/api/v1/auth/forgot', { email }) });
}
export function useCheckReset(token: string) {
  return useQuery({ queryKey: ['reset-check', token], queryFn: async () => { await api.post<void>('/api/v1/auth/reset/check', { token }); return true; }, retry: false, enabled: !!token, staleTime: Infinity });
}
export function useReset() {
  return useMutation({ mutationFn: (v: { token: string; password: string }) => api.post<void>('/api/v1/auth/reset', v) });
}

export function useIntegrations() {
  return useQuery({ queryKey: ['integrations'], queryFn: async () => (await api.get<{ items: IntegrationRow[] }>('/api/v1/integrations')).items, refetchInterval: 15_000, retry: false });
}
export function useSetIntegrationEnabled() {
  const qc = useQueryClient();
  return useMutation({ mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) => api.patch<void>(`/api/v1/integrations/${id}`, { enabled }), onSuccess: () => inv(qc, 'integrations', 'contracts') });
}
export function useRotateIntegrationToken() {
  const qc = useQueryClient();
  return useMutation({ mutationFn: (id: string) => api.post<{ token: string }>(`/api/v1/integrations/${id}/token`), onSuccess: () => inv(qc, 'integrations') });
}

export function useSettings() {
  return useQuery({ queryKey: ['settings'], queryFn: () => api.get<PlatformSettings>('/api/v1/settings'), retry: false });
}
export function useSaveSettings() {
  const qc = useQueryClient();
  return useMutation({ mutationFn: (platform_name: string) => api.patch<void>('/api/v1/settings', { platform_name }), onSuccess: () => inv(qc, 'settings') });
}
export function useTagList() {
  return useQuery({ queryKey: ['tag-list'], queryFn: async () => (await api.get<{ items: TagRow[] }>('/api/v1/tags')).items, retry: false });
}
export function useSaveTag() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...v }: { id?: string; name: string; color: string }) => (id ? api.patch<TagRow>(`/api/v1/tags/${id}`, v) : api.post<TagRow>('/api/v1/tags', v)),
    onSuccess: () => inv(qc, 'tag-list', 'tags', 'assets'),
  });
}
export function useDeleteTag() {
  const qc = useQueryClient();
  return useMutation({ mutationFn: (id: string) => api.del(`/api/v1/tags/${id}`), onSuccess: () => inv(qc, 'tag-list', 'tags', 'assets') });
}

export function usePeople(q: string) {
  return useQuery({ queryKey: ['people', q], queryFn: async () => (await api.get<{ items: PersonRow[] }>(`/api/v1/people${q ? `?q=${encodeURIComponent(q)}` : ''}`)).items, retry: false });
}
export function useSavePerson() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...v }: { id?: string; version?: number; first_name: string; last_name: string; email: string | null; organization: string | null }) =>
      id ? api.patch<PersonRow>(`/api/v1/people/${id}`, v) : api.post<PersonRow>('/api/v1/people', v),
    onSuccess: () => inv(qc, 'people', 'teams', 'team'),
  });
}
export function useDeletePerson() {
  const qc = useQueryClient();
  return useMutation({ mutationFn: (id: string) => api.del(`/api/v1/people/${id}`), onSuccess: () => inv(qc, 'people', 'teams', 'team') });
}
export function useTeams() {
  return useQuery({ queryKey: ['teams'], queryFn: async () => (await api.get<{ items: TeamRow[] }>('/api/v1/teams')).items, retry: false });
}
export function useTeam(id: string | undefined) {
  return useQuery({ queryKey: ['team', id], enabled: !!id, queryFn: () => api.get<TeamDetail>(`/api/v1/teams/${id}`), retry: false });
}
export function useSaveTeam() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...v }: { id?: string; version?: number; name: string; description: string | null; kind: string }) =>
      id ? api.patch<TeamDetail>(`/api/v1/teams/${id}`, v) : api.post<TeamDetail>('/api/v1/teams', v),
    onSuccess: () => inv(qc, 'teams', 'team', 'people'),
  });
}
export function useDeleteTeam() {
  const qc = useQueryClient();
  return useMutation({ mutationFn: (id: string) => api.del(`/api/v1/teams/${id}`), onSuccess: () => inv(qc, 'teams', 'team', 'people') });
}
export function useSetMembers(id: string) {
  const qc = useQueryClient();
  return useMutation({ mutationFn: (v: { version: number; person_ids: string[] }) => api.put<TeamDetail>(`/api/v1/teams/${id}/members`, v), onSuccess: () => inv(qc, 'teams', 'team', 'people') });
}

export function useMitreMatrix() {
  return useQuery({ queryKey: ['mitre'], queryFn: () => api.get<MitreMatrix>('/api/v1/mitre/matrix'), retry: false, refetchInterval: (q) => (q.state.data?.last_run?.status === 'running' ? 3000 : false) });
}
export function useMitreTechnique(id: string | null) {
  return useQuery({ queryKey: ['mitre-technique', id], enabled: !!id, queryFn: () => api.get<MitreDetail>(`/api/v1/mitre/techniques/${id}`), retry: false });
}
export function useMitreImport() {
  const qc = useQueryClient();
  return useMutation({ mutationFn: () => api.post<void>('/api/v1/mitre/import'), onSuccess: () => inv(qc, 'mitre') });
}

export function useAlerts(unreadOnly = false, limit = 30, enabled = true, organization = '') {
  return useQuery({
    enabled, queryKey: ['alerts', unreadOnly, limit, organization],
    queryFn: () => api.get<AlertList>(`/api/v1/alerts?limit=${limit}${unreadOnly ? '&unread=1' : ''}${organization ? `&organization_id=${encodeURIComponent(organization)}` : ''}`),
    refetchInterval: 10_000,
    retry: false,
  });
}
export function useAlertAction() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, action }: { id?: string; action: 'read' | 'dismiss' | 'read-all' }) => api.post<void>(action === 'read-all' ? '/api/v1/alerts/read-all' : `/api/v1/alerts/${id}/${action}`),
    onSuccess: () => inv(qc, 'alerts'),
  });
}

// ---------- Fase 4 ----------
export function useAgents(enabled = true) {
  return useQuery({ enabled, queryKey: ['agents'], queryFn: async () => (await api.get<{ items: AgentRow[] }>('/api/v1/agents')).items, refetchInterval: 10_000, retry: false });
}
export function useCreateEnrollmentToken() {
  return useMutation({ mutationFn: (v: { os: string; arch: string; mode: string; organization_id?: string }) => api.post<EnrollmentToken>('/api/v1/agents/enrollment-tokens', v) });
}
export function useEnrollmentStatus(id: string | undefined) {
  const qc = useQueryClient();
  return useQuery({
    queryKey: ['enrollment', id],
    enabled: !!id,
    queryFn: async () => {
      const s = await api.get<EnrollmentStatus>(`/api/v1/agents/enrollment-tokens/${id}`);
      if (s.used) await Promise.all([qc.invalidateQueries({ queryKey: ['agents'] }), qc.invalidateQueries({ queryKey: ['assets'] }), qc.invalidateQueries({ queryKey: ['organization-assets'] })]);
      return s;
    },
    refetchInterval: (q) => (q.state.data?.used || q.state.data?.expired ? false : 2000),
    retry: false,
  });
}
export function useRevokeAgent() {
  const qc = useQueryClient();
  return useMutation({ mutationFn: (id: string) => api.post<void>(`/api/v1/agents/${id}/revoke`), onSuccess: () => Promise.all([qc.invalidateQueries({ queryKey: ['agents'] }), qc.invalidateQueries({ queryKey: ['assets'] })]) });
}

// ---------- T4: data endpoint (osquery) ----------
export function useEndpointCatalog() {
  return useQuery({ queryKey: ['osquery-catalog'], queryFn: () => api.get<EndpointCatalog>('/api/v1/osquery/catalog'), staleTime: 5 * 60_000, retry: false });
}
export function useOrgOsquery(orgId: string, enabled = true) {
  return useQuery({ queryKey: ['osquery-org', orgId], enabled, queryFn: () => api.get<{ items: OrgOsquerySetting[] }>(`/api/v1/organizations/${orgId}/osquery`), retry: false });
}
export function useSetOrgOsquery(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { key: string; enabled: boolean; version: number; consent?: Pick<EndpointConsent, 'version' | 'sha256'> }) => api.put<void>(`/api/v1/organizations/${orgId}/osquery/${v.key}`, { enabled: v.enabled, version: v.version, consent: v.consent }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['osquery-org', orgId] }),
  });
}
export function useSetOrgPack(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { pack: string; enabled: boolean; versions: Record<string,number>; consent?: Pick<EndpointConsent, 'version' | 'sha256'> }) => api.put<void>(`/api/v1/organizations/${orgId}/osquery-packs/${v.pack}`, { enabled: v.enabled, versions: v.versions, consent: v.consent }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['osquery-org', orgId] }),
  });
}
export function useAssetEndpoint(assetId: string, enabled = true) {
  const connected = useEndpointStream(assetId, enabled);
  return useQuery({ queryKey: ['asset-endpoint', assetId], enabled, queryFn: () => api.get<EndpointAssetData>(`/api/v1/assets/${assetId}/endpoint-data`), retry: false, refetchInterval: connected ? false : 60_000 });
}
export function useRefreshEndpoint(assetId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (queryKey: string) => api.post<unknown>(`/api/v1/assets/${assetId}/endpoint-data/refresh`, { query_key: queryKey }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['asset-endpoint', assetId] }); window.setTimeout(() => qc.invalidateQueries({ queryKey: ['asset-endpoint', assetId] }), 4000); },
  });
}

export function useDeleteEndpointData(assetId: string) {
 const qc=useQueryClient();
 return useMutation({mutationFn:(version:number)=>api.del(`/api/v1/assets/${assetId}/endpoint-data?version=${version}`),onSuccess:()=>{qc.invalidateQueries({queryKey:['asset-endpoint',assetId]});qc.invalidateQueries({queryKey:['asset',assetId]});}});
}
