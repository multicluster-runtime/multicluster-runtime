#! /usr/bin/env bash
# Copyright 2025 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit

LATEST_TAG=$(git describe --tags --abbrev=0 --match="v[0-9]*" HEAD)

find "providers" -name go.mod | while read -r GOMOD; do
    module=$(dirname "${GOMOD}")
    if [ "$(git tag -l "${module}/${LATEST_TAG}")" ]; then
        echo "${module}/${LATEST_TAG} already exists. Do you really need to run this script?"
        exit 1
    fi

    pushd "${module}"
    go get "sigs.k8s.io/multicluster-runtime@${LATEST_TAG}"
    go mod tidy
    popd
done

git add ./providers/
git commit -m "Update providers to sigs.k8s.io/multicluster-runtime@${LATEST_TAG}"

if [ -n "$PROVIDERS_GIT_PUSH" ]; then
    git push origin
fi

find "providers" -name go.mod | while read -r GOMOD; do
    module=$(dirname "${GOMOD}")
    git tag -am "${module}/${LATEST_TAG}" "${module}/${LATEST_TAG}"
    if [ -n "$PROVIDERS_GIT_PUSH" ]; then
        git push origin "${module}/${LATEST_TAG}"
    fi
done


