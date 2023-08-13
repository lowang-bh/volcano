APP=scheduler
echo $#, $1
if [ $# -ge 1 ]; then
    echo "checking parameters"
    if [ $1 == "scheduler" ] || [ $1 == "controller-manager" ] || [ $1 == "webhook-manager" ] ;then
        APP=$1
    fi
fi

REPO=volcanosh
VERSION=$(git describe  --always)
#GitSHA=$(git rev-parse HEAD)
GitSHA=$(git log -1 --pretty=format:%h)
Branch=$(git rev-parse --abbrev-ref HEAD)
Date=$(date "+%Y-%m-%d %H:%M:%S")
RELEASE_VER=${Branch}-${GitSHA}
ImageName=${REPO}/vc-${APP}:${RELEASE_VER}
echo "build image: ${ImageName}"
LD_FLAGS=" \
    -X '${REPO_PATH}/pkg/version.GitSHA=${GitSHA}' \
    -X '${REPO_PATH}/pkg/version.Built=${Date}'   \
    -X '${REPO_PATH}/pkg/version.Version=${RELEASE_VER}'"

echo "build with LD_FLAGS: ${LD_FLAGS}"

BIN_DIR=_output/bin

GO111MODULE=on CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "${LD_FLAGS}" -o ${BIN_DIR}/vc-${APP} ./cmd/${APP}
docker build  . -f Dockerfile.${APP} -t "${ImageName}"

kind load docker-image ${ImageName}  ${ImageName} --name cluster-v1.27

kubectl patch deployments.apps -n volcano-system volcano-scheduler --patch "{\"spec\": {\"template\": {\"spec\": {\"containers\": [{\"name\": \"volcano-scheduler\",\"image\": \"${ImageName}\"}]}}}}"