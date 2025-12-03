#!/bin/bash
set -e

TARGET="/ban/strats"
SOURCE="/ban/strats_init"

# 判断宿主机挂载目录是否为空
if [ -z "$(ls -A $TARGET)" ]; then
    echo "banstrats dir empty → initializing with container default files..."
    cp -r $SOURCE/* $TARGET/
fi

# 检查配置文件是否存在
if [ ! -f "/ban/data/config.yml" ] && [ ! -f "/ban/data/config.local.yml" ]; then
    echo "running bot init..."
    /ban/bot init
fi

# 运行原本的 CMD
exec "$@"
