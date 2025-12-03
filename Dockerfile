FROM golang:1.25.4

ENV BanDataDir=/ban/data
ENV BanStratDir=/ban/strats

WORKDIR /ban/strats_init

COPY . .

RUN git reset --hard HEAD && git pull origin main && \
  go mod tidy && \
  go build -o ../bot

RUN chmod +x /ban/bot && /ban/bot init && \
  chmod +x /ban/strats_init/scripts/run.sh

EXPOSE 8000 8001

WORKDIR /ban/strats

ENTRYPOINT ["/ban/strats_init/scripts/run.sh", "/ban/bot"]
