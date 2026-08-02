/** Delivery channel for a notification rule. */
export type NotificationChannel = 'slack' | 'email';

/** Event name that can trigger a notification. */
export type NotificationEventName = 'run_failed' | 'flaky';

/** Status of a notification delivery attempt. */
export type NotificationDeliveryStatus = 'pending' | 'delivered' | 'failed';

/** A notification rule that fires when specific events occur. */
export interface NotificationRule {
	/** Wire-format id, e.g. nrule_... */
	id: string;
	channel: NotificationChannel;
	target: string;
	on: NotificationEventName[];
	/** ISO 8601 UTC timestamp. */
	created_at: string;
}

/** A recorded delivery attempt for a notification rule. */
export interface NotificationDelivery {
	/** Wire-format id, e.g. ndel_... */
	id: string;
	/** Wire-format rule id. */
	rule_id: string;
	channel: NotificationChannel;
	status: NotificationDeliveryStatus;
	response_code: number | null;
	attempt_count: number;
	error_message: string | null;
	/** ISO 8601 UTC timestamp. */
	attempted_at: string;
}

/** Request body for creating a notification rule. */
export interface CreateRuleRequest {
	channel: NotificationChannel;
	target: string;
	on: NotificationEventName[];
}
