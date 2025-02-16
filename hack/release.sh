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

VERSION="${1:-}"
if [ -z "${VERSION}" ]; then
  echo "Usage: $0 <version>"
  exit 1
fi

function stepi () {
  printf '        '
  printf ' \033[1;37m%q\033[0m' "$@"
  printf '\n'
  read -p "Run? [Ysq] " -n1 INPUT
  echo
  if [ "${INPUT}" == "s" ]; then
    echo "Skipping..."
    return
  elif [ "${INPUT}" == "y" ] || [ "${INPUT}" == "Y" ] || [ "${INPUT}" == "" ]; then
    "$@"
    return
  else
    echo "Exiting..."
    exit 1
  fi
}

function step () {
  printf '        '
  printf ' \033[1;37m%q\033[0m' "$@"
  printf '\n'
  "$@"
}

stepi git tag -s -m "Tag of ${VERSION}" "${VERSION}"
stepi git push origin "${VERSION}"

for DIR in providers examples; do
  for GOMOD in $(find "${DIR}" -name go.mod); do
    B=$(dirname ${GOMOD})
    if [ "$(uname)" = "Darwin" ]; then
      stepi sed -i '' -E '/github\.com\/multicluster-runtime\/multicluster-runtime/ s/([[:space:]]+)v[[:alnum:]._+\-]+$/\1'"${VERSION}"'/' "${B}/go.mod"
    else
      stepi sed -i -E '/github\.com\/multicluster-runtime\/multicluster-runtime/ s/([[:space:]]+)v[[:alnum:]._+\-]+$/\1'"${VERSION}"'/' "${B}/go.mod"
    fi
    step git diff "${B}/go.mod"
    echo
    pushd ${B}
    stepi go mod tidy
    popd
    stepi git add "${B}/go.mod"
    stepi git commit --allow-empty -m "Update ${B}/go.mod to depend on ${VERSION}"
    stepi git tag -s -m "$V" "${B}/${VERSION}"
    stepi git push origin "${B}/${VERSION}"
  done
done

stepi git push

echo "Congratulations! You have released ${VERSION}! 🎉"
echo
echo "Now don't forget to:"
echo "  * Create and push a release branch."
echo "  * Release the tag as a GitHub release."
