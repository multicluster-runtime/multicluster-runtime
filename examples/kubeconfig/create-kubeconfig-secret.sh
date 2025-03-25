#!/bin/bash

# Script to create a kubeconfig secret for the Multicluster Failover Operator

set -e

# Default values
NAMESPACE="my-operator-namespace"
KUBECONFIG_PATH="${HOME}/.kube/config"
KUBECONFIG_CONTEXT=""
SECRET_NAME=""
DRY_RUN="false"

# Function to display usage information
function show_help {
  echo "Usage: $0 [options]"
  echo "  -n, --name NAME         Name for the secret (will be used as cluster identifier)"
  echo "  -s, --namespace NS      Namespace to create the secret in (default: ${NAMESPACE})"
  echo "  -k, --kubeconfig PATH   Path to kubeconfig file (default: ${KUBECONFIG_PATH})"
  echo "  -c, --context CONTEXT   Kubeconfig context to use (default: current-context)"
  echo "  -d, --dry-run           Dry run, print YAML but don't apply"
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

# Process the kubeconfig
echo "Processing kubeconfig..."
TEMP_KUBECONFIG=$(mktemp)
trap "rm -f $TEMP_KUBECONFIG" EXIT

if [ -n "$KUBECONFIG_CONTEXT" ]; then
  kubectl config view --raw --minify --flatten --context="$KUBECONFIG_CONTEXT" > "$TEMP_KUBECONFIG"
  if [ $? -ne 0 ]; then
    echo "ERROR: Failed to extract context '$KUBECONFIG_CONTEXT' from kubeconfig"
    exit 1
  fi
  echo "Extracted context '$KUBECONFIG_CONTEXT' from kubeconfig"
else
  cp "$KUBECONFIG_PATH" "$TEMP_KUBECONFIG"
  echo "Using entire kubeconfig file"
fi

# Encode the kubeconfig
KUBECONFIG_B64=$(base64 < "$TEMP_KUBECONFIG" | tr -d '\n')

# Create the namespace if it doesn't exist
if [ "$DRY_RUN" != "true" ]; then
  kubectl get namespace "$NAMESPACE" &>/dev/null || kubectl create namespace "$NAMESPACE"
fi

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

# Apply or print the secret
if [ "$DRY_RUN" == "true" ]; then
  echo "# YAML that would be applied:"
  echo "$SECRET_YAML"
else
  echo "$SECRET_YAML" | kubectl apply -f -
  echo "Secret '${SECRET_NAME}' created in namespace '${NAMESPACE}'"
  echo "The operator should now discover and connect to this cluster"
fi 