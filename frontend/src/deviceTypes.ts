export type LedgerDevice = {
  channel_id: string
  channel_name: string
  server_id: number
  name: string
  business_node_id?: string
  business_node_name?: string
  profile_id?: string
  vendor?: string
  model?: string
  protocol: string
  address?: string
  point_count: number
  command_count: number
  health: string
  success_rate: number
  avg_latency_ms: number
  verified: boolean
  verifier?: string
  verified_at?: string
  device_serial?: string
}

export type UnitModelConfig = {
  name: string
  profile_id: string
  business_node_id?: string
  point_map: Record<string, string>
  verified: boolean
  verifier?: string
  verified_at?: string
  device_serial?: string
  verification?: string
}

export type UnitPoint = {
  id: string
  name: string
  server_id: number
  address: number
  io_type: string
  read_only: boolean
  unit?: string
  scale: number
}

export type UnitParameter = {
  name: string
  label: string
  point_id?: string
  point_name: string
  unit?: string
  kind?: string
  has_range?: boolean
  min: number
  max: number
  default: number
  available: boolean
  reason?: string
}

export type UnitCommand = {
  id: string
  name: string
  point_id?: string
  point_name: string
  value: number
  parameters?: UnitParameter[]
  available: boolean
  reason?: string
}

export type ProfileSpec = {
  id: string
  vendor: string
  model: string
  protocol: string
  verified: boolean
  verification?: string
  notes?: string[]
  custom?: boolean
  point_specs?: { name: string; unit?: string; read_only?: boolean; role?: string }[]
  command_specs?: { id: string; name: string }[]
}

export type UnitModel = {
  channel_id: string
  server_id: number
  model?: UnitModelConfig
  profiles: ProfileSpec[]
  profile_errors?: string[]
  points: UnitPoint[]
  commands: UnitCommand[]
  missing?: string[]
}

export type StateValue = {
  device_id: string
  point_id: string
  value: { kind: string; number?: number; bool?: boolean; enum?: string }
  quality: string
  source_time: string
  received_time: string
}

export type CommandOutcome = {
  request_id: string
  device_id: string
  command_id: string
  actor: string
  state: string
  reason?: string
  started_at: string
  updated_at: string
}
