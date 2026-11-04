import {t} from './runtime';
const keys = {
"release_unavailable":"api_release_unavailable" as const,
"agent_revoked":"api_agent_revoked" as const,
"account_email_exists":"api_account_email_exists" as const,
"own_role_change":"api_own_role_change" as const,
"password_too_short":"api_password_too_short" as const,
"current_password_wrong":"api_current_password_wrong" as const,
"person_email_exists":"api_person_email_exists" as const,
"address_ip_invalid":"api_address_ip_invalid" as const,
"address_cidr_invalid":"api_address_cidr_invalid" as const,
"address_hostname_invalid":"api_address_hostname_invalid" as const,
"address_url_invalid":"api_address_url_invalid" as const,
"address_mac_invalid":"api_address_mac_invalid" as const,
"address_kind_invalid":"api_address_kind_invalid" as const,
"addresses_too_many":"api_addresses_too_many" as const,

dependency_cycle:"api_dependency_cycle" as const,
"invalid_request": "api_invalid_request" as const,
"unauthorized": "api_unauthorized" as const,
"forbidden": "api_forbidden" as const,
"not_found": "api_not_found" as const,
"conflict": "api_conflict" as const,
"payload_too_large": "api_payload_too_large" as const,
"validation_failed": "api_validation_failed" as const,
"field_invalid": "api_field_invalid" as const,
"rate_limited": "api_rate_limited" as const,
"request_failed": "api_request_failed" as const,
"name_invalid": "api_name_invalid" as const,
"file_empty": "api_file_empty" as const,
"ports_invalid": "api_ports_invalid" as const,
"testing_not_authorized": "api_testing_not_authorized" as const,
"agent_offline": "api_agent_offline" as const,
"collection_conflict": "api_collection_conflict" as const,
"asset_conflict": "api_asset_conflict" as const,
"already_running": "api_already_running" as const,
"language_invalid": "api_language_invalid" as const,
"invalid_credentials": "api_invalid_credentials" as const,
"email_invalid": "api_email_invalid" as const,
"asset_no_agent": "api_asset_no_agent" as const,
"step_asset_no_agent": "api_step_asset_no_agent" as const,
};
export function apiErrorText(status:number,code?:string):string {
 if(status>=500)return t(keys.request_failed);
 const key=code&&Object.prototype.hasOwnProperty.call(keys,code)?keys[code as keyof typeof keys]:undefined;
 if(key)return t(key);
 switch(status){case 400:return t(keys.invalid_request);case 401:return t(keys.unauthorized);case 403:return t(keys.forbidden);case 404:return t(keys.not_found);case 409:return t(keys.conflict);case 413:return t(keys.payload_too_large);case 422:return t(keys.validation_failed);case 429:return t(keys.rate_limited);default:return t(keys.invalid_request)}
}
