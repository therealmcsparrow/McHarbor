-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

UPDATE plugin_reporting_schedules
   SET report_type = 'workload-inventory', updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
 WHERE report_type = 'container-inventory';

UPDATE plugin_reporting_schedules
   SET report_type = 'workload-inventory', subject_kind = '', subject_id = '', subject_name = '',
       updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
 WHERE report_type IN ('kvm-inventory', 'lxc-inventory', 'compute-inventory',
                       'kubernetes-workload-inventory', 'swarm-service-inventory');

UPDATE plugin_reporting_schedules
   SET report_type = 'data-protection-rpo', subject_kind = '', subject_id = '', subject_name = '',
       updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
 WHERE report_type IN ('kvm-protection', 'lxc-protection');

UPDATE plugin_reporting_schedules
   SET report_type = 'capacity-utilisation', subject_kind = '', subject_id = '', subject_name = '',
       updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
 WHERE report_type IN ('compute-capacity', 'lxc-utilisation', 'kvm-allocation');

UPDATE plugin_reporting_schedules
   SET report_type = 'environment-health', subject_kind = '', subject_id = '', subject_name = '',
       updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
 WHERE report_type IN ('kubernetes-cluster-health', 'swarm-cluster-health');

UPDATE plugin_reporting_reports
   SET report_type = 'workload-inventory',
       document = CASE WHEN json_valid(document)
                       THEN json_set(document, '$.meta.report_type', 'workload-inventory')
                       ELSE document END,
       updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
 WHERE report_type = 'container-inventory';

UPDATE plugin_reporting_schedule_runs
   SET document_json = json_set(document_json, '$.meta.report_type', 'workload-inventory')
 WHERE json_valid(document_json)
   AND json_extract(document_json, '$.meta.report_type') = 'container-inventory';

UPDATE plugin_reporting_reports
   SET status = 'retired', updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
 WHERE report_type IN ('kvm-inventory', 'lxc-inventory', 'compute-inventory',
                       'kubernetes-workload-inventory', 'swarm-service-inventory',
                       'kvm-protection', 'lxc-protection', 'compute-capacity',
                       'lxc-utilisation', 'kvm-allocation',
                       'kubernetes-cluster-health', 'swarm-cluster-health');
