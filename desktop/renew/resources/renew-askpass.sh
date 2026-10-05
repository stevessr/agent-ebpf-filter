#!/bin/sh
# sudo invokes this as the ordinary user and privately reads stdout. Never
# log the response or pass it as an argument to another process.
if command -v zenity >/dev/null 2>&1; then
    exec zenity --password --title="Renew 后端授权"
elif command -v kdialog >/dev/null 2>&1; then
    exec kdialog --title "Renew 后端授权" --password "请输入登录密码以启动 eBPF 后端："
fi
echo "Renew needs zenity/kdialog, a configured SUDO_ASKPASS, or a desktop PolicyKit agent." >&2
exit 1
