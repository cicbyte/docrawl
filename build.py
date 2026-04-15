from pathlib import Path
import subprocess
import logging as log
import os
import shutil
import sys
import time
log.basicConfig(level=log.INFO, format="%(asctime)s - %(levelname)s - %(message)s")

# 全局配置
USE_GOX = False

def run_command(cmd, cwd=None, shell=False, direct_output=False):
    """运行命令并实时显示输出
    Args:
        cmd: 要运行的命令
        cwd: 工作目录
        shell: 是否使用shell执行
        direct_output: 是否直接输出到终端（用于显示进度条等）
    """
    if direct_output:
        # 直接输出到终端，用于显示进度条等
        process = subprocess.Popen(
            cmd,
            stdout=sys.stdout,
            stderr=sys.stderr,
            cwd=cwd,
            shell=shell,
            universal_newlines=True
        )
        return_code = process.wait()
        return return_code
    else:
        # 通过logging输出
        process = subprocess.Popen(
            cmd,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            encoding='utf-8',
            errors='replace',
            cwd=cwd,
            shell=shell,
            bufsize=1,
            universal_newlines=True
        )
        
        # 实时读取并显示输出
        while True:
            output = process.stdout.readline()
            if output == '' and process.poll() is not None:
                break
            if output:
                log.info(output.strip())
        
        # 获取返回码
        return_code = process.poll()
        
        # 检查是否有错误输出
        stderr = process.stderr.read()
        if stderr:
            log.error(stderr.strip())
        
        return return_code

def run_npm_build():
    target_dir = Path(__file__).parent.joinpath("web")
    target_dir_path = target_dir.resolve()
    log.info("target_dir_path: " + str(target_dir_path))
    
    # 在Windows系统上使用npm.cmd
    npm_cmd = "npm.cmd" if os.name == 'nt' else "npm"
    
    try:
        return_code = run_command(
            [npm_cmd, "run", "build"],
            cwd=target_dir_path,
            shell=True
        )
        
        if return_code == 0:
            log.info("npm run build 命令执行成功")
        else:
            log.error("npm run build 命令执行失败")
            raise Exception("npm run build 命令执行失败")
    except Exception as e:
        log.error(f"执行npm命令时出错: {str(e)}")
        raise

def remove_old_build():
    target_dir = Path(__file__).parent.joinpath("resources").joinpath("static")
    
    # 检查并创建目标目录
    try:
        target_dir.mkdir(parents=True, exist_ok=True)
    except Exception as e:
        log.error(f"创建目录失败: {str(e)}")
        raise
    
    # 删除目录内容（包括子目录）
    for item in target_dir.glob('*'):
        try:
            if item.is_file():
                item.unlink()
            elif item.is_dir():
                shutil.rmtree(item)
        except Exception as e:
            log.error(f"删除 {item} 失败: {str(e)}")
            raise

def copy_new_build():
    source_dir = Path(__file__).parent.joinpath("web").joinpath("dist")
    target_dir = Path(__file__).parent.joinpath("resources").joinpath("static")
    
    # 确保源目录存在
    if not source_dir.exists():
        log.error("源目录不存在: " + str(source_dir))
        raise FileNotFoundError("源目录不存在")
    
    # 复制整个目录结构
    try:
        shutil.copytree(
            source_dir,
            target_dir,
            dirs_exist_ok=True  # 允许目标目录已存在
        )
        log.info(f"成功复制文件从 {source_dir} 到 {target_dir}")
    except Exception as e:
        log.error(f"复制文件失败: {str(e)}")
        raise

def check_upx():
    """检查UPX是否可用"""
    try:
        return_code = run_command(["upx", "--version"])
        return return_code == 0
    except FileNotFoundError:
        return False

def compress_with_upx(output_name):
    """使用UPX压缩可执行文件"""
    if not check_upx():
        log.info("UPX未安装，跳过压缩步骤")
        return
    
    try:
        # 获取压缩前文件大小
        original_size = os.path.getsize(output_name)
        log.info(f"开始UPX压缩，原始文件大小: {original_size/1024/1024:.2f}MB")
        
        # 执行UPX压缩，使用--verbose选项显示详细信息，并直接输出到终端以显示进度条
        return_code = run_command(
            ["upx", "--best", "--verbose", output_name],
            direct_output=True
        )
        
        if return_code == 0:
            # 获取压缩后文件大小
            compressed_size = os.path.getsize(output_name)
            # 计算压缩率
            compression_ratio = (1 - compressed_size / original_size) * 100
            log.info(f"压缩完成: {original_size/1024/1024:.2f}MB -> {compressed_size/1024/1024:.2f}MB (压缩率: {compression_ratio:.1f}%)")
            
            # 清理UPX临时文件
            base_name = os.path.splitext(output_name)[0]  # 移除.exe扩展名
            temp_files = [
                f"{base_name}.000",
                f"{base_name}.upx"
            ]
            for temp_file in temp_files:
                if os.path.exists(temp_file):
                    try:
                        os.remove(temp_file)
                        log.info(f"已清理临时文件: {temp_file}")
                    except Exception as e:
                        log.warning(f"清理临时文件 {temp_file} 失败: {str(e)}")
        else:
            log.error("UPX压缩失败")
    except Exception as e:
        log.error(f"UPX压缩时出错: {str(e)}")

def check_gox():
    """检查gox是否已安装"""
    try:
        # gox 没有 -version 参数，直接运行 gox 命令检查是否可用
        return_code = run_command(["gox", "-h"])
        return return_code == 0
    except FileNotFoundError:
        return False

def get_version():
    """获取版本号
    优先使用 Git 标签作为版本号，如果没有标签则使用 Git 提交哈希
    """
    try:
        # 尝试获取最新的 Git 标签
        tag = subprocess.check_output(
            ["git", "describe", "--tags", "--abbrev=0"],
            stderr=subprocess.DEVNULL,
            universal_newlines=True
        ).strip()
        return tag
    except subprocess.CalledProcessError:
        try:
            # 如果没有标签，使用 Git 提交哈希
            commit = subprocess.check_output(
                ["git", "rev-parse", "--short", "HEAD"],
                stderr=subprocess.DEVNULL,
                universal_newlines=True
            ).strip()
            return f"0.0.0-{commit}"
        except subprocess.CalledProcessError:
            # 如果 Git 命令失败，返回默认版本号
            return "0.0.0-dev"

def get_build_info():
    """获取构建信息"""
    try:
        # 获取 Git 提交哈希
        commit = subprocess.check_output(
            ["git", "rev-parse", "HEAD"],
            stderr=subprocess.DEVNULL,
            universal_newlines=True
        ).strip()
        
        # 获取 Git 分支名
        branch = subprocess.check_output(
            ["git", "rev-parse", "--abbrev-ref", "HEAD"],
            stderr=subprocess.DEVNULL,
            universal_newlines=True
        ).strip()
        
        return {
            "commit": commit,
            "branch": branch,
            "build_time": time.strftime("%Y-%m-%d %H:%M:%S")
        }
    except subprocess.CalledProcessError:
        return {
            "commit": "unknown",
            "branch": "unknown",
            "build_time": time.strftime("%Y-%m-%d %H:%M:%S")
        }

def build_with_gox():
    """使用gox进行交叉编译"""
    if not check_gox():
        log.error("gox未安装，请先安装gox: go install github.com/mitchellh/gox@v1.0.1")
        raise Exception("gox未安装")
    
    try:
        # 设置输出目录
        output_dir = "build"
        
        # 清理输出目录
        if os.path.exists(output_dir):
            shutil.rmtree(output_dir)
        os.makedirs(output_dir)
        
        # 获取版本信息和构建信息
        version = get_version()
        build_info = get_build_info()
        
        # 设置编译参数
        ldflags = (
            f"-X 'main.Version={version}' "
            f"-X 'main.BuildTime={build_info['build_time']}' "
            f"-X 'main.GitCommit={build_info['commit']}' "
            f"-X 'main.GitBranch={build_info['branch']}'"
        )
        
        # 执行gox命令，在项目根目录下执行
        return_code = run_command([
            "gox",
            "-os", "windows linux darwin",
            "-arch", "amd64",
            "-ldflags", ldflags,
            "-output", "%s/docrawl_windows_" % output_dir
        ])
        
        if return_code == 0:
            log.info(f"gox交叉编译成功，版本: {version}")
            
            # 为Windows版本添加.exe扩展名
            for file in os.listdir(output_dir):
                if file.startswith("docrawl_windows") and not file.endswith(".exe"):
                    old_path = os.path.join(output_dir, file)
                    new_path = old_path + ".exe"
                    os.rename(old_path, new_path)
                    log.info(f"重命名Windows可执行文件: {file} -> {file}.exe")
            
            # 只对当前平台的可执行文件进行UPX压缩
            current_os = "windows" if os.name == 'nt' else "linux" if os.name == 'posix' else "darwin"
            for file in os.listdir(output_dir):
                if file.startswith(f"aibookmark_{current_os}"):
                    file_path = os.path.join(output_dir, file)
                    if os.path.isfile(file_path):
                        log.info(f"开始压缩当前平台文件: {file}")
                        compress_with_upx(file_path)
                    else:
                        log.info(f"跳过非可执行文件: {file}")
                else:
                    log.info(f"跳过其他平台文件: {file}")
        else:
            log.error("gox交叉编译失败")
            raise Exception("gox交叉编译失败")
    except Exception as e:
        log.error(f"使用gox编译时出错: {str(e)}")
        raise

def build_go_app():
    """编译Go应用"""
    try:
        if USE_GOX:
            build_with_gox()
            return
            
        # 设置输出文件名
        output_name = "docrawl.exe" if os.name == 'nt' else "docrawl"
        
        # 执行go build命令
        return_code = run_command(["go", "build", "-o", output_name])
        
        if return_code == 0:
            log.info("Go应用编译成功")
            # 尝试使用UPX压缩
            compress_with_upx(output_name)
        else:
            log.error("Go应用编译失败")
            raise Exception("Go应用编译失败")
    except Exception as e:
        log.error(f"编译Go应用时出错: {str(e)}")
        raise

def main():
    run_npm_build()
    remove_old_build()
    copy_new_build()
    build_go_app()

if __name__ == "__main__":
    main()