#!/bin/bash

# --- 配置部分 ---
# 检测目标 IP (建议使用国内稳定 IP，如阿里云 DNS)
TARGET_IP="8.8.8.8"
TARGET_PORT="53"

# 日志存放目录 (脚本会自动创建这个文件夹)
LOG_DIR="/home/navitech/network-monitor-logs"

# 1. 检测网络
# -z: 扫描模式 (不发送数据)
# -w 5: 超时时间设置为 5 秒
nc -z -w 5 "$TARGET_IP" "$TARGET_PORT" > /dev/null 2>&1

# 2. 判断结果
if [ $? != 0 ]; then
    # === 网络断开，执行以下操作 ===

    # 获取当前时间戳，例如: 20231027_1230
    CURRENT_TIME=$(date +%Y%m%d_%H%M)

    # 确保日志目录存在
    if [ ! -d "$LOG_DIR" ]; then
        mkdir -p "$LOG_DIR"
    fi

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

    # 为了防止写入未完成就断电/重启，执行 sync
    sync

    # --- 执行重启 ---
    /sbin/shutdown -r now
else
    # 网络正常，退出
    exit 0
fi