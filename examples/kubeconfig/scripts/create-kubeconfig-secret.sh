#!/bin/bash

# Script to create a kubeconfig secret and RBAC rules for the Operator

set -e

# Default values
NAMESPACE="my-operator-namespace"
KUBECONFIG_PATH="${HOME}/.kube/config"
KUBECONFIG_CONTEXT=""
SECRET_NAME=""
DRY_RUN="false"
VERIFY="true"
OUTPUT_FILE=""
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RBAC_DIR="$(cd "${SCRIPT_DIR}/../rbac" && pwd)"

# Function to process a YAML template file
function process_template() {
    local template_file="$1"
    if [ ! -f "$template_file" ]; then
        echo "ERROR: Template file not found: $template_file"
        exit 1
    fi
    
    # Export variables for envsubst
    export SECRET_NAME
    export NAMESPACE
    
    # Use envsubst to replace variables
    envsubst < "$template_file"
}

# Function to display usage information
function show_help {
  echo "Usage: $0 [options]"
  echo "  -n, --name NAME         Name for the secret (will be used as cluster identifier)"
  echo "  -s, --namespace NS      Namespace to create the secret in (default: ${NAMESPACE})"
  echo "  -k, --kubeconfig PATH   Path to kubeconfig file (default: ${KUBECONFIG_PATH})"
  echo "  -c, --context CONTEXT   Kubeconfig context to use (default: current-context)"
  echo "  -d, --dry-run           Dry run, print YAML but don't apply"
  echo "  --no-verify             Skip verification step"
  echo "  -o, --output FILE       Save the generated kubeconfig to a file"
  echo "  -h, --help              Show this help message"
  echo ""
  echo "Example: $0 -n cluster1 -c prod-cluster -k ~/.kube/config"
}

# Parse command line options
while [[ $# -gt 0 ]]; do
  key="$1"
  case $key in
    -n|--name)
      SECRET_NAME="$2"
      shift 2
      ;;
    -s|--namespace)
      NAMESPACE="$2"
      shift 2
      ;;
    -k|--kubeconfig)
      KUBECONFIG_PATH="$2"
      shift 2
      ;;
    -c|--context)
      KUBECONFIG_CONTEXT="$2"
      shift 2
      ;;
    -d|--dry-run)
      DRY_RUN="true"
      shift 1
      ;;
    --no-verify)
      VERIFY="false"
      shift 1
      ;;
    -o|--output)
      OUTPUT_FILE="$2"
      shift 2
      ;;
    -h|--help)
      show_help
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      show_help
      exit 1
      ;;
  esac
done

# Validate required arguments
if [ -z "$SECRET_NAME" ]; then
  echo "ERROR: Secret name is required (-n, --name)"
  show_help
  exit 1
fi

if [ ! -f "$KUBECONFIG_PATH" ]; then
  echo "ERROR: Kubeconfig file not found at: $KUBECONFIG_PATH"
  exit 1
fi

# Create the namespace if it doesn't exist
if [ "$DRY_RUN" != "true" ]; then
  kubectl get namespace "$NAMESPACE" &>/dev/null || kubectl create namespace "$NAMESPACE"
fi

# Process and combine RBAC templates
RBAC_YAML=$(cat <<EOF
---
$(process_template "${RBAC_DIR}/serviceaccount.yaml")
---
$(process_template "${RBAC_DIR}/clusterrole.yaml")
---
$(process_template "${RBAC_DIR}/clusterrolebinding.yaml")
EOF
)

# Apply RBAC rules first
if [ "$DRY_RUN" != "true" ]; then
  echo "Applying RBAC rules..."
  echo "$RBAC_YAML" | kubectl apply -f -
  
  # Wait for service account to be created and get its token
  echo "Waiting for service account to be created..."
  sleep 2  # Give the API server time to create the service account
  
  # Get the cluster CA certificate
  CLUSTER_CA=$(kubectl config view --raw --minify --flatten -o jsonpath='{.clusters[].cluster.certificate-authority-data}')
  if [ -z "$CLUSTER_CA" ]; then
    echo "ERROR: Could not get cluster CA certificate"
    exit 1
  fi
  
  # Get the cluster server URL
  CLUSTER_SERVER=$(kubectl config view --raw --minify --flatten -o jsonpath='{.clusters[].cluster.server}')
  if [ -z "$CLUSTER_SERVER" ]; then
    echo "ERROR: Could not get cluster server URL"
    exit 1
  fi
  
  # Get the service account token
  SA_TOKEN=$(kubectl -n ${NAMESPACE} create token ${SECRET_NAME}-sa --duration=8760h)
  if [ -z "$SA_TOKEN" ]; then
    echo "ERROR: Could not create service account token"
    exit 1
  fi
  
  # Create a new kubeconfig using the service account token
  NEW_KUBECONFIG=$(cat <<EOF
apiVersion: v1
kind: Config
clusters:
- name: ${SECRET_NAME}
  cluster:
    server: ${CLUSTER_SERVER}
    certificate-authority-data: ${CLUSTER_CA}
contexts:
- name: ${SECRET_NAME}
  context:
    cluster: ${SECRET_NAME}
    user: ${SECRET_NAME}-sa
current-context: ${SECRET_NAME}
users:
- name: ${SECRET_NAME}-sa
  user:
    token: ${SA_TOKEN}
EOF
)
  
  # Save kubeconfig to file if requested
  if [ -n "$OUTPUT_FILE" ]; then
    echo "Saving kubeconfig to ${OUTPUT_FILE}..."
    echo "$NEW_KUBECONFIG" > "$OUTPUT_FILE"
    echo "Kubeconfig saved to ${OUTPUT_FILE}"
  fi
  
  # Encode the new kubeconfig
  KUBECONFIG_B64=$(echo "$NEW_KUBECONFIG" | base64 -w0)
  
  # Generate the secret YAML
  SECRET_YAML=$(cat <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: ${SECRET_NAME}
  namespace: ${NAMESPACE}
  labels:
    sigs.k8s.io/multicluster-runtime-kubeconfig: "true"
type: Opaque
data:
  kubeconfig: ${KUBECONFIG_B64}
EOF
)
  
  echo "Creating kubeconfig secret..."
  echo "$SECRET_YAML" | kubectl apply -f -
  
  echo "Secret '${SECRET_NAME}' created in namespace '${NAMESPACE}'"
  echo "RBAC rules created for service account '${SECRET_NAME}-sa'"
  
  if [ "$VERIFY" == "true" ]; then
    echo "Verifying setup..."
    
    # Check if secret exists
    if ! kubectl get secret "${SECRET_NAME}" -n "${NAMESPACE}" &>/dev/null; then
      echo "ERROR: Secret ${SECRET_NAME} not found in namespace ${NAMESPACE}"
      exit 1
    fi
    
    # Check if service account exists
    if ! kubectl get serviceaccount "${SECRET_NAME}-sa" -n "${NAMESPACE}" &>/dev/null; then
      echo "ERROR: Service account ${SECRET_NAME}-sa not found in namespace ${NAMESPACE}"
      exit 1
    fi
    
    # Check if cluster role exists
    if ! kubectl get clusterrole "${SECRET_NAME}-role" &>/dev/null; then
      echo "ERROR: Cluster role ${SECRET_NAME}-role not found"
      exit 1
    fi
    
    # Check if cluster role binding exists
    if ! kubectl get clusterrolebinding "${SECRET_NAME}-rolebinding" &>/dev/null; then
      echo "ERROR: Cluster role binding ${SECRET_NAME}-rolebinding not found"
      exit 1
    fi
    
    # Verify the token works
    echo "Verifying token permissions..."
    if ! kubectl --token="${SA_TOKEN}" get pods -A &>/dev/null; then
      echo "ERROR: Service account token does not have required permissions"
      exit 1
    fi
    
    echo "Setup verified successfully!"
  fi
else
  echo "# RBAC YAML that would be applied:"
  echo "$RBAC_YAML"
  echo ""
  echo "# Example of the kubeconfig that would be created (with redacted values):"
  echo "apiVersion: v1"
  echo "kind: Config"
  echo "clusters:"
  echo "- name: ${SECRET_NAME}"
  echo "  cluster:"
  echo "    server: <cluster-server-url>"
  echo "    certificate-authority-data: <cluster-ca>"
  echo "contexts:"
  echo "- name: ${SECRET_NAME}"
  echo "  context:"
  echo "    cluster: ${SECRET_NAME}"
  echo "    user: ${SECRET_NAME}-sa"
  echo "current-context: ${SECRET_NAME}"
  echo "users:"
  echo "- name: ${SECRET_NAME}-sa"
  echo "  user:"
  echo "    token: <service-account-token>"
fi

echo "The operator should now be able to discover and connect to this cluster" 