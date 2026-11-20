#!/bin/bash

# 网络看门狗：连续多次探测都失败才重启设备。
#
# 由 cron/systemd timer 周期性调用。单次探测失败不能说明网络真的断了（丢包、目标
# 临时不可达、DNS 服务限流都会造成），而每次重启都会中断采集、丢掉内存里尚未落库的
# 状态，所以这里要求：所有目标、连续 PROBE_ROUNDS 轮都探测失败，才认为网络已断。

# --- 配置部分（都可以用同名环境变量覆盖）---
# 探测目标，空格分隔的 "IP:端口" 列表，任意一个可达即视为网络正常。
# 建议加入本站点实际使用的同步服务器地址；局域网站点请改成局域网内的地址。
TARGETS="${TARGETS:-8.8.8.8:53 223.5.5.5:53}"

# 连续探测的轮数与两轮之间的间隔（秒）。
PROBE_ROUNDS="${PROBE_ROUNDS:-3}"
PROBE_INTERVAL="${PROBE_INTERVAL:-20}"

# 日志存放目录 (脚本会自动创建这个文件夹)，以及最多保留的诊断文件个数。
LOG_DIR="${LOG_DIR:-/home/navitech/network-monitor-logs}"
MAX_LOG_FILES="${MAX_LOG_FILES:-20}"

# 没有 nc 时无法判断网络状态：直接退出，不能把"命令不存在"当成"网络断开"去重启。
if ! command -v nc >/dev/null 2>&1; then
    echo "network-monitor: nc not found, skipping check" >&2
    exit 0
fi

# 1. 检测网络
# -z: 扫描模式 (不发送数据)
# -w 5: 超时时间设置为 5 秒
network_ok() {
    local target host port
    for target in $TARGETS; do
        host="${target%:*}"
        port="${target##*:}"
        if nc -z -w 5 "$host" "$port" >/dev/null 2>&1; then
            return 0
        fi
    done
    return 1
}

round=1
while [ "$round" -le "$PROBE_ROUNDS" ]; do
    if network_ok; then
        # 网络正常，退出
        exit 0
    fi
    if [ "$round" -lt "$PROBE_ROUNDS" ]; then
        sleep "$PROBE_INTERVAL"
    fi
    round=$((round + 1))
done

# 2. === 所有轮次都失败：网络断开，执行以下操作 ===

# 获取当前时间戳，例如: 20231027_1230
CURRENT_TIME=$(date +%Y%m%d_%H%M)

# 确保日志目录存在
mkdir -p "$LOG_DIR"

# 定义 raspinfo 保存的文件名
RASPINFO_FILE="$LOG_DIR/raspinfo_$CURRENT_TIME.txt"

# --- 关键步骤：保存 raspinfo ---
# 注意：raspinfo 可能需要一点时间运行
# 检查系统中是否有 raspinfo 命令
if command -v raspinfo >/dev/null 2>&1; then
    raspinfo > "$RASPINFO_FILE" 2>&1
else
    echo "Error: raspinfo command not found" > "$RASPINFO_FILE"
    dmesg >> "$RASPINFO_FILE" # 如果没有raspinfo，至少保存dmesg
fi

# 只保留最近的 MAX_LOG_FILES 个诊断文件，长时间断网时不会把磁盘写满。
ls -1t "$LOG_DIR"/raspinfo_*.txt 2>/dev/null | tail -n +"$((MAX_LOG_FILES + 1))" | while IFS= read -r old; do
    rm -f -- "$old"
done

# 为了防止写入未完成就断电/重启，执行 sync
sync

# --- 执行重启 ---
/sbin/shutdown -r now
