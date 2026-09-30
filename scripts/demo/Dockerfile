FROM debian:stable-slim
RUN apt-get update && apt-get install -y --no-install-recommends tmux asciinema curl ca-certificates procps git locales && rm -rf /var/lib/apt/lists/* \
 && sed -i 's/# en_US.UTF-8/en_US.UTF-8/' /etc/locale.gen && locale-gen
ENV LANG=en_US.UTF-8 LC_ALL=en_US.UTF-8
RUN curl -fsSL -o /usr/local/bin/agg https://github.com/asciinema/agg/releases/latest/download/agg-x86_64-unknown-linux-gnu && chmod +x /usr/local/bin/agg
