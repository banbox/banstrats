FROM golang:1.25.4

ENV BanDataDir=/ban/data
ENV BanStratDir=/ban/strats

WORKDIR /ban/strats

COPY . .

RUN git reset --hard HEAD && git pull origin main && \
  go mod tidy && \
  go build -o ../bot

RUN chmod +x /ban/bot && /ban/bot init

EXPOSE 8000 8001

ENTRYPOINT ["/ban/bot"]
