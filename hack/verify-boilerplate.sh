set -euo pipefail

cd $(dirname $0)/..
source hack/lib.sh


echodate "Checking KubeFlag licenses..."
boilerplate \
  -boilerplates hack/boilerplate \
  -exclude .github