source /nfs/lsf/conf/profile.lsf;
source /nfs/go/src/github.com/dgruber/drmaa/examples/simplesubmit/newBuild.sh;
make build CGO_ENABLED=1 CC=gcc CXX=g++ GOOS=linux GOARCH=amd64 BUILD_PLATFORMS="-os=linux -arch=amd64";
