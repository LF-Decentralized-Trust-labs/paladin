#!/bin/bash

# A script to dump logs and diagnostic information from a Kubernetes cluster
# for Paladin and Besu deployments.
#
# This script is intended to be called by a CI/CD system when a task fails.
#
# Usage:
#   ./dump_logs.sh <context> <paladin_namespace> [besu_namespace]

set -e
set -u
set -o pipefail

# --- Helper Functions ---

# Fetches the names of StatefulSets based on an app label.
# Arguments:
#   $1: namespace
#   $2: app_label (e.g., "paladin" or "besu")
get_node_names() {
    local namespace="$1"
    local app_label="$2"
    # The '|| true' prevents the script from exiting if no resources are found.
    kubectl get statefulset -n "${namespace}" -l "app=${app_label}" -o jsonpath='{.items[*].metadata.name}' || true
}

# Dumps logs for all pods in a given StatefulSet.
# Arguments:
#   $1: namespace
#   $2: log_dir
#   $3: app_label (e.g., "paladin" or "besu")
dump_node_logs() {
    local namespace="$1"
    local log_dir="$2"
    local app_label="$3"

    echo "--- Dumping ${app_label} node logs from namespace [${namespace}] ---"
    local node_names
    node_names=$(get_node_names "${namespace}" "${app_label}")

    if [[ -z "${node_names}" ]]; then
        echo "No ${app_label} nodes found in namespace ${namespace}."
        return
    fi

    for node_name in ${node_names}; do
        local log_file="${log_dir}/${app_label}-${node_name}.log"
        echo "Saving ${node_name} logs to ${log_file}"
        kubectl logs "statefulset/${node_name}" -n "${namespace}" > "${log_file}" 2>/dev/null || echo "Warning: Could not get logs for ${node_name}."
    done
}

# Dumps Paladin CRs and associated ConfigMaps.
# Arguments:
#   $1: namespace
#   $2: log_dir
dump_crds_and_configmaps() {
    local namespace="$1"
    local log_dir="$2"

    echo "--- Dumping all Paladin Custom Resources ---"
    local all_crds_file="${log_dir}/paladin-custom-resources.yaml"
    kubectl get paladin -n "${namespace}" -o yaml > "${all_crds_file}" 2>/dev/null || echo "Warning: Could not list Paladin CRs."

    echo "--- Dumping Paladin ConfigMaps ---"
    local paladin_nodes
    paladin_nodes=$(get_node_names "${namespace}" "paladin")

    if [[ -z "${paladin_nodes}" ]]; then
        echo "No Paladin nodes found for dumping ConfigMaps."
        return
    fi

    for node_name in ${paladin_nodes}; do
        # Dump ConfigMap
        local cm_file="${log_dir}/cm-${node_name}.json"
        echo "Saving ${node_name} ConfigMap to ${cm_file}"
        kubectl get cm "${node_name}" -n "${namespace}" -o json > "${cm_file}" 2>/dev/null || echo "Warning: Could not get ConfigMap for ${node_name}."
    done
}

# Dumps general cluster status.
# Arguments:
#   $1: namespace
#   $2: log_dir
dump_cluster_status() {
    local namespace="$1"
    local log_dir="$2"

    echo "--- Dumping cluster status for namespace [${namespace}] ---"
    local status_file="${log_dir}/cluster-status-${namespace}.txt"
    echo "Saving cluster status to ${status_file}"
    kubectl get all,reg,scd,txn -n "${namespace}" -o wide > "${status_file}" 2>/dev/null || echo "Warning: Could not get cluster status."
}

# Dumps pprof and other debug information from Paladin pods.
# Arguments:
#   $1: namespace
#   $2: log_dir
dump_pprof_info() {
    local namespace="$1"
    local log_dir="$2"

    echo "--- Dumping pprof and debug info ---"
    local paladin_nodes
    paladin_nodes=$(get_node_names "${namespace}" "paladin")

    if [[ -z "${paladin_nodes}" ]]; then
        echo "No Paladin nodes found for dumping pprof info."
        return
    fi

    for node_name in ${paladin_nodes}; do
        local pod_name="${node_name}-0" # Assumes StatefulSet pod naming convention

        # Dump goroutine profile
        local pprof_file="${log_dir}/pprof-goroutine-${node_name}.txt"
        echo "Saving ${node_name} pprof goroutine dump to ${pprof_file}"
        kubectl exec "${pod_name}" -n "${namespace}" -- curl -s "http://localhost:6060/debug/pprof/goroutine?debug=2" > "${pprof_file}" 2>/dev/null || echo "Warning: Could not get pprof info for ${pod_name}."
    done
}


# --- Main Function ---

main() {
    if [[ "$#" -lt 2 || "$#" -gt 3 ]]; then
        echo "Usage: $0 <context> <paladin_namespace> [besu_namespace]"
        exit 1
    fi

    local context="$1"
    local paladin_namespace="$2"
    # Default to paladin_namespace if the third argument is not provided
    local besu_namespace="${3:-${paladin_namespace}}"

    local timestamp
    timestamp=$(date +%Y%m%d_%H%M%S)
    local log_dir="dump-${paladin_namespace}-${timestamp}"

    echo "Dumping logs and diagnostic information in: ${context}"

    # Create log directory
    mkdir -p "${log_dir}"
    echo "Created log directory: ${log_dir}"

    # Wrap dumping in a try-catch block to ensure we report completion
    (
        # Dump operator logs (assumes operator is in paladin namespace)
        echo "--- Dumping operator logs ---"
        local operator_log_file="${log_dir}/operator.log"
        echo "Saving operator logs to ${operator_log_file}"
        kubectl logs deployment/paladin-operator -n "${paladin_namespace}" > "${operator_log_file}" 2>/dev/null || echo "Warning: Could not get operator logs."

        # Dump logs for Paladin and Besu nodes from their respective namespaces
        dump_node_logs "${paladin_namespace}" "${log_dir}" "paladin"
        dump_node_logs "${besu_namespace}" "${log_dir}" "besu"

        # Dump CRDs and ConfigMaps from the Paladin namespace
        dump_crds_and_configmaps "${paladin_namespace}" "${log_dir}"

        # Dump cluster status for both namespaces if they are different
        dump_cluster_status "${paladin_namespace}" "${log_dir}"
        if [[ "${paladin_namespace}" != "${besu_namespace}" ]]; then
            dump_cluster_status "${besu_namespace}" "${log_dir}"
        fi

        # Dump pprof information from the Paladin namespace
        dump_pprof_info "${paladin_namespace}" "${log_dir}"
    ) || {
        echo "An error occurred during log dumping. Some files may be missing."
    }

    echo ""
    echo "All logs and diagnostic information saved to: ${log_dir}"
    echo "Upload this directory as an artifact in your CI/CD pipeline."
}

main "$@"
