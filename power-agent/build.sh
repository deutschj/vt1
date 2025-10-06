#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "Usage: $0 <version> [--latest] [--arch arm64|arm] [--push]"
  echo "Examples:"
  echo "  $0 v0.1.0 --arch arm64 --push"
  echo "  $0 2025-10-04_01 --latest --arch arm --push"
  exit 1
}

VERSION="${1:-}"; shift || true
[[ -z "${VERSION}" ]] && usage

TAG_LATEST="false"
ARCH="arm64"
DO_PUSH="false"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --latest) TAG_LATEST="true"; shift ;;
    --arch)   ARCH="${2:-}"; shift 2 ;;
    --push)   DO_PUSH="true"; shift ;;
    *) echo "Unknown arg: $1"; usage ;;
  esac
done

REPO="juliandeutsch/raspi-power-agent"
IMAGE_TAG="${REPO}:${VERSION}"

echo "Building for GOARCH=${ARCH}..."
docker build --build-arg GOARCH="${ARCH}" -t "${IMAGE_TAG}" .

IMAGE_ID="$(docker images -q "${IMAGE_TAG}")"
if [[ -z "${IMAGE_ID}" ]]; then
  echo "Failed to obtain image ID for ${IMAGE_TAG}" >&2
  exit 2
fi

echo "Built ${IMAGE_TAG}"
echo "Image ID: ${IMAGE_ID}"

if [[ "${TAG_LATEST}" == "true" ]]; then
  docker tag "${IMAGE_TAG}" "${REPO}:latest"
  echo "Tagged ${REPO}:latest -> ${IMAGE_ID}"
fi

if [[ "${DO_PUSH}" == "true" ]]; then
  echo "Pushing ${IMAGE_TAG}..."
  docker push "${IMAGE_TAG}"
  if [[ "${TAG_LATEST}" == "true" ]]; then
    echo "Pushing ${REPO}:latest..."
    docker push "${REPO}:latest"
  fi
else
  echo "(Skipping push; pass --push to push to Docker Hub)"
  echo "If needed: docker push ${IMAGE_TAG}"
  [[ "${TAG_LATEST}" == "true" ]] && echo "          docker push ${REPO}:latest"
fi

