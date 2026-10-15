/**
 * `cmd/main`（启停脚本）的模板。
 *
 * 为什么单独成模块：这份脚本是**项目能不能被应用中心启停**的关键——start / stop / status
 * 三个子命令要按飞牛的约定写（status 返回 0 = 在跑、3 = 没跑），自己从零写很容易漏掉
 * pid 文件、僵尸 pid、TERM 之后等不到退出这些边角，表现是「应用中心里的启动按钮点了没反应，
 * 或者停了之后状态还是运行中」。
 *
 * docker 项目和非 docker 项目是**两套完全不同的写法**，所以按项目类型给不同的模板
 * （`mainScriptTemplate`）：非 docker 的由脚本自己托管进程（写 pid 文件、发 TERM / KILL），
 * 而 docker 应用是应用中心拉起的容器，脚本什么都不用做，只要 status 去问 docker 那个容器。
 *
 * 两份都是**官方脚手架的原样文本**，不要顺手「优化」里面的写法：用户会拿它跟飞牛的文档、
 * 跟别人的项目对照，改一个字就对不上了。模板只写进编辑器（插进去之后要不要保存由用户决定），
 * 落地时不碰用户的文件。
 */

/**
 * 非 docker 项目：脚本自己启动 / 停止进程。
 *
 * `CMD` 是留给用户填的启动命令（模板里是空的），进程 pid 写进 `${TRIM_PKGVAR}/app.pid`、
 * 日志追加到 `${TRIM_PKGVAR}/info.log`；`TRIM_PKGVAR` 是安装器注入的数据目录变量。
 */
export const MAIN_SCRIPT_TEMPLATE = `#!/bin/bash

LOG_FILE="\${TRIM_PKGVAR}/info.log"
PID_FILE="\${TRIM_PKGVAR}/app.pid"

# write the command to start your program here 
CMD=""

log_msg() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') - $1" >> \${LOG_FILE}
}

start_process() {
    if status; then
        return 0
    fi

    log_msg "Starting process ..."
    # run cmd to start process
    bash -c "\${CMD}" >> \${LOG_FILE} 2>&1 &
    # write pid to pidfile
    printf "%s" "$!" > \${PID_FILE}
    # log_msg "CMD = \${CMD}"
    # log_msg "pid = $!"
    return 0
}

stop_process() {
    log_msg "Stopping process ..."

    if [ -r "\${PID_FILE}" ]; then
        pid=$(head -n 1 "\${PID_FILE}" | tr -d '[:space:]')
        
        log_msg "pid=\${pid}"
        if ! check_process "\${pid}"; then
            # process not exist, delete pidfile
            rm -f "\${PID_FILE}"
            log_msg "remove pid file 1"
            return
        fi

        log_msg "send TERM signal to PID:\${pid}..."
        kill -TERM \${pid} >> \${LOG_FILE} 2>&1

        local count=0
        while check_process "\${pid}" && [ $count -lt 10 ]; do
            sleep 1
            count=$((count + 1))
            log_msg "waiting process terminal... (\${count}s/10s)"
        done

        if check_process "\${pid}"; then
            log_msg "send KILL signal to PID:\${pid}..."
            kill -KILL "\${pid}"
            sleep 1
            rm -f "\${PID_FILE}"
        else
            log_msg "process killed... "
        fi
    fi

    return 0
}

check_process() {
    local pid=$1
    if kill -0 "\${pid}" 2>/dev/null; then
        return 0  # process exist
    else
        return 1  # process not exist
    fi
}

status() {
    if [ -f "\${PID_FILE}" ]; then
        pid=$(head -n 1 "\${PID_FILE}" | tr -d '[:space:]')
        if check_process "\${pid}"; then
            return 0
        else
            # Process is not running but pidfile exists - clean it up
            rm -f "\${PID_FILE}"
        fi    
    fi

    return 1
}

case $1 in
start)
    # run start command. exit 0 if success, exit 1 if failed
    start_process
    ;;
stop)
    # run stop command. exit 0 if success, exit 1 if failed
    stop_process
    ;;
status)
    # check application status command. exit 0 if running, exit 3 if not running
    if status; then 
        exit 0
    else 
        exit 3
    fi
    ;;
*)
    exit 1
    ;;
esac
`

/**
 * docker 项目：容器由应用中心拉起来，脚本只管 status。
 *
 * 容器名从 `${TRIM_APPDEST}/docker/docker-compose.yaml` 的 `container_name` 读
 * （`TRIM_APPDEST` 是应用自己的安装目录）；start / stop 直接 `exit 0`——应用中心会自己
 * `docker compose up / down`，脚本再动一次只会跟它打架。
 */
export const MAIN_DOCKER_SCRIPT_TEMPLATE = `#!/bin/bash

FILE_PATH="\${TRIM_APPDEST}/docker/docker-compose.yaml"

is_docker_running () {
    DOCKER_NAME=""

    if [ -f "$FILE_PATH" ]; then
        DOCKER_NAME=$(cat $FILE_PATH | grep "container_name" | awk -F ':' '{print $2}' | xargs)
        echo "DOCKER_NAME is set to: $DOCKER_NAME"
    fi

    if [ -n "$DOCKER_NAME" ]; then
        docker inspect $DOCKER_NAME | grep -q "\\"Status\\": \\"running\\"," || exit 1
        return
    fi
}

case $1 in
start)
    # run start command. exit 0 if success, exit 1 if failed
    # do nothing, docker application will be started by appcenter
    exit 0
    ;;
stop)
    # run stop command. exit 0 if success, exit 1 if failed
    # do nothing, docker application will be stopped by appcenter
    exit 0
    ;;
status)
    # check application status command. exit 0 if running, exit 3 if not running
    # check first container by default, you cound modify it by yourself 
    if is_docker_running; then
        exit 0
    else
        exit 3
    fi
    ;;
*)
    exit 1
    ;;
esac
`

/**
 * 按项目类型挑启停脚本模板。
 *
 * 判据只有「是不是 docker 项目」这一条（`ProjectInfo.docker`，后端按
 * `app/docker/docker-compose.yaml` 是否存在判定），所以这里只收一个布尔值：调用方把
 * 「项目信息还没读回来」这种情况交给那个布尔值自己的默认值（非 docker），模板层不再猜一遍。
 */
export function mainScriptTemplate(docker: boolean): string {
  return docker ? MAIN_DOCKER_SCRIPT_TEMPLATE : MAIN_SCRIPT_TEMPLATE
}
