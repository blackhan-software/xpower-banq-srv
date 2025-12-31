FROM denoland/deno:alpine
ENV VERSION=v10.0.0

RUN apk add --no-cache coreutils supercronic

ADD https://github.com/blackhan-software/xpower-banq-cli/releases/download/v1.0.0/banq-mainnet.x86_64-linux.run \
    /usr/local/bin/banq
RUN echo "a0ff3681be773b30893201943962d9188f26d1808e051015682b75c786dc2898 /usr/local/bin/banq" | sha256sum -c - && \
    chmod +x /usr/local/bin/banq

COPY docker/xpowermine/miner/mine.sh /usr/local/bin/mine.sh
RUN chmod +x /usr/local/bin/mine.sh

COPY docker/xpowermine/miner/mine.cron /tmp/mine.cron
RUN crontab /tmp/mine.cron

CMD ["sh", "-c", "/usr/local/bin/mine.sh & exec /usr/sbin/crond -f"]
