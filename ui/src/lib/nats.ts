// Subsets of the native NATS JetStream JSON types natsmin passes through.

export interface Placement {
	cluster?: string;
	tags?: string[];
	preferred?: string;
}

export interface StreamSource {
	name: string;
	opt_start_seq?: number;
	opt_start_time?: string;
	filter_subject?: string;
	external?: { api: string; deliver?: string };
	domain?: string;
}

export interface StreamConfig {
	name: string;
	description?: string;
	subjects?: string[];
	retention?: 'limits' | 'interest' | 'workqueue';
	max_consumers?: number;
	max_msgs?: number;
	max_bytes?: number;
	discard?: 'old' | 'new';
	discard_new_per_subject?: boolean;
	max_age?: number;
	max_msgs_per_subject?: number;
	max_msg_size?: number;
	storage?: 'file' | 'memory';
	num_replicas?: number;
	no_ack?: boolean;
	duplicate_window?: number;
	placement?: Placement;
	mirror?: StreamSource;
	sources?: StreamSource[];
	sealed?: boolean;
	deny_delete?: boolean;
	deny_purge?: boolean;
	allow_rollup_hdrs?: boolean;
	compression?: 's2' | 'none';
	first_seq?: number;
	allow_direct?: boolean;
	mirror_direct?: boolean;
	allow_msg_ttl?: boolean;
	subject_delete_marker_ttl?: number;
	metadata?: Record<string, string>;
	[k: string]: unknown;
}

export interface PeerInfo {
	name: string;
	current: boolean;
	offline?: boolean;
	active: number;
	lag?: number;
}

export interface ClusterInfo {
	name?: string;
	leader?: string;
	replicas?: PeerInfo[];
}

export interface StreamState {
	messages: number;
	bytes: number;
	first_seq: number;
	first_ts: string;
	last_seq: number;
	last_ts: string;
	consumer_count: number;
	deleted?: number[];
	num_deleted?: number;
	num_subjects?: number;
	subjects?: Record<string, number>;
}

export interface StreamInfo {
	config: StreamConfig;
	created: string;
	state: StreamState;
	cluster?: ClusterInfo;
	mirror?: { name: string; lag: number; active: number };
	sources?: { name: string; lag: number; active: number }[];
	ts?: string;
}

export interface ConsumerConfig {
	name?: string;
	durable_name?: string;
	description?: string;
	deliver_policy?: 'all' | 'last' | 'new' | 'by_start_sequence' | 'by_start_time' | 'last_per_subject';
	opt_start_seq?: number;
	opt_start_time?: string;
	ack_policy?: 'none' | 'all' | 'explicit';
	ack_wait?: number;
	max_deliver?: number;
	filter_subject?: string;
	filter_subjects?: string[];
	replay_policy?: 'instant' | 'original';
	sample_freq?: string;
	max_waiting?: number;
	max_ack_pending?: number;
	headers_only?: boolean;
	max_batch?: number;
	max_expires?: number;
	inactive_threshold?: number;
	num_replicas?: number;
	mem_storage?: boolean;
	deliver_subject?: string;
	deliver_group?: string;
	flow_control?: boolean;
	idle_heartbeat?: number;
	backoff?: number[];
	pause_until?: string;
	metadata?: Record<string, string>;
	[k: string]: unknown;
}

export interface SequenceInfo {
	consumer_seq: number;
	stream_seq: number;
	last_active?: string;
}

export interface ConsumerInfo {
	stream_name: string;
	name: string;
	created: string;
	config: ConsumerConfig;
	delivered: SequenceInfo;
	ack_floor: SequenceInfo;
	num_ack_pending: number;
	num_redelivered: number;
	num_waiting: number;
	num_pending: number;
	cluster?: ClusterInfo;
	push_bound?: boolean;
	paused?: boolean;
	pause_remaining?: number;
	ts?: string;
}

export interface AccountInfo {
	memory: number;
	storage: number;
	reserved_memory?: number;
	reserved_storage?: number;
	streams: number;
	consumers: number;
	domain?: string;
	api: { level?: number; total: number; errors: number };
	limits: {
		max_memory: number;
		max_storage: number;
		max_streams: number;
		max_consumers: number;
		max_ack_pending?: number;
		memory_max_stream_bytes?: number;
		storage_max_stream_bytes?: number;
		max_bytes_required?: boolean;
	};
	tiers?: Record<string, unknown>;
}
