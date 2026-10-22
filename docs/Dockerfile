# Sphinx toolchain for building the proxy's documentation.
# All Python tooling lives in this image; nothing is installed on the host.
FROM python:3.14-slim@sha256:a2b82f3c48559aa0a8446d9af49826b6e2b2016f4cd2afabfe6013ec53729170

RUN apt-get update \
    && apt-get install --yes --no-install-recommends make \
    && rm -rf /var/lib/apt/lists/*

COPY requirements.txt /tmp/requirements.txt
RUN pip install --no-cache-dir --root-user-action=ignore -r /tmp/requirements.txt

# The container runs as the calling user (see compose.yaml), who has no home directory.
ENV HOME=/tmp \
    PYTHONDONTWRITEBYTECODE=1

WORKDIR /docs

CMD ["make", "html"]
