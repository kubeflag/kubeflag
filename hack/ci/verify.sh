#!/usr/bin/env bash

#                Kubermatic Enterprise Read-Only License
#                       Version 1.0 ("KERO-1.0")
#                   Copyright © 2023 Kubermatic GmbH
#
# 1.	You may only view, read and display for studying purposes the source
#    code of the software licensed under this license, and, to the extent
#    explicitly provided under this license, the binary code.
# 2.	Any use of the software which exceeds the foregoing right, including,
#    without limitation, its execution, compilation, copying, modification
#    and distribution, is expressly prohibited.
# 3.	THE SOFTWARE IS PROVIDED “AS IS”, WITHOUT WARRANTY OF ANY KIND,
#    EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
#    MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
#    IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
#    CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
#    TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
#    SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
#
# END OF TERMS AND CONDITIONS

### This script is used as a presubmit to check that Helm chart versions
### have been updated if charts have been modified. Without the Prow env
### vars, this script won't run properly.

set -euo pipefail

cd $(dirname $0)/../..
source hack/lib.sh

EXIT_CODE=0

try() {
  local title="$1"
  shift

  heading "$title"
  echo -e "$@\n"

  start_time=$(date +%s)

  set +e
  $@
  exitCode=$?
  set -e

  elapsed_time=$(($(date +%s) - $start_time))
  TEST_NAME="$title" write_junit $exitCode "$elapsed_time"

  if [[ $exitCode -eq 0 ]]; then
    echo -e "\n[${elapsed_time}s] SUCCESS :)"
  else
    echo -e "\n[${elapsed_time}s] FAILED."
    EXIT_CODE=1
  fi

  git reset --hard --quiet
  git clean --force

  echo
}

try "Verify license compatibility" ./hack/verify-licenses.sh
try "Verify Go import statements" ./hack/verify-import-order.sh
try "Verify boilerplate" ./hack/verify-boilerplate.sh

exit $EXIT_CODE