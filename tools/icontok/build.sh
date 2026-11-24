#!/bin/sh
# Build the icontok oracle against a private copy of the reference Icon tree.
#
#   tools/icontok/build.sh [ICON_SRC] [WORKDIR]
#
# ICON_SRC defaults to ../icon (relative to the repo root); it is copied,
# never modified. WORKDIR (default: $TMPDIR/icontok-build) holds the copy.
# Prints the path of the built binary; point $ICONTOK at it for go test.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
src=${1:-$here/../../../icon}
work=${2:-${TMPDIR:-/tmp}/icontok-build}
case $(uname) in Darwin) cfg=macintosh ;; *) cfg=linux ;; esac

if [ ! -f "$work/src/icont/tparse.o" ]; then
  rm -rf "$work"
  mkdir -p "$work"
  (cd "$src" && tar --exclude=.git -cf - .) | (cd "$work" && tar xf -)
  (cd "$work" && make Configure name=$cfg >/dev/null 2>&1)
  (cd "$work/src/common" && make >/dev/null 2>&1)
  (cd "$work/src/icont" && make >/dev/null 2>&1)
fi
cd "$work/src/icont"
objs=$(ls *.o | grep -v -e '^tunix\.o$' -e '^tcode\.o$')
common=$(ls ../common/*.o | grep -v rswitch)
cc -w -I. -o icontok "$here/icontok.c" $objs $common -lm 2>&1
echo "$work/src/icont/icontok"
