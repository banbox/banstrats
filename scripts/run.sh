#!/bin/bash
set -e

TARGET="/ban/strats"
SOURCE="/ban/strats_init"

# 判断宿主机挂载目录是否为空
if [ -z "$(ls -A $TARGET)" ]; then
    echo "banstrats dir empty → initializing with container default files..."
    cp -r $SOURCE/* $TARGET/
fi

# 运行原本的 CMD
exec "$@"
