#!/usr/bin/env python3
"""
将所有 Markdown 文件中的代码块语言标识设置为 python

使用方法:
  python set_code_python.py <target_dir>
  python set_code_python.py ./output
  python set_code_python.py D:/docs
"""

import argparse
import re
import sys
from pathlib import Path


def process_markdown_file(file_path: Path) -> tuple[int, int]:
    """
    处理单个 Markdown 文件
    返回: (修改的代码块数, 总代码块数)
    """
    try:
        content = file_path.read_text(encoding='utf-8')
    except Exception as e:
        print(f"  ❌ 读取失败: {e}")
        return 0, 0

    original_content = content
    modified_blocks = 0
    total_blocks = 0

    # 找到所有 ``` 的位置，成对处理（奇数个是开始，偶数个是结束）
    lines = content.split('\n')
    result_lines = []
    in_code_block = False

    for line in lines:
        stripped = line.strip()
        if stripped.startswith('```'):
            if not in_code_block:
                # 这是代码块开始
                total_blocks += 1
                # 提取现有语言标识
                lang = stripped[3:].strip().split()[0] if len(stripped) > 3 else ''

                if lang.lower() != 'python':
                    modified_blocks += 1
                    # 获取原有缩进
                    indent = line[:len(line) - len(line.lstrip())]
                    result_lines.append(f"{indent}```python")
                else:
                    result_lines.append(line)
                in_code_block = True
            else:
                # 这是代码块结束，保持原样
                result_lines.append(line)
                in_code_block = False
        else:
            result_lines.append(line)

    new_content = '\n'.join(result_lines)

    if new_content != original_content:
        try:
            file_path.write_text(new_content, encoding='utf-8')
        except Exception as e:
            print(f"  ❌ 写入失败: {e}")
            return 0, total_blocks

    return modified_blocks, total_blocks


def main():
    parser = argparse.ArgumentParser(
        description='将 Markdown 文件中的代码块语言标识设置为 python',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog='''
示例:
  python set_code_python.py ./output
  python set_code_python.py D:/docs
  python set_code_python.py ../my-docs
'''
    )
    parser.add_argument(
        'target',
        type=str,
        nargs='?',
        default='./output',
        help='目标文件夹路径 (默认: ./output)'
    )

    args = parser.parse_args()
    target_dir = Path(args.target).resolve()

    if not target_dir.exists():
        print(f"❌ 目录不存在: {target_dir}")
        sys.exit(1)

    if not target_dir.is_dir():
        print(f"❌ 不是文件夹: {target_dir}")
        sys.exit(1)

    print(f"📁 处理目录: {target_dir}")
    print()

    total_files = 0
    total_modified_blocks = 0
    total_blocks = 0

    # 遍历所有 .md 文件
    for md_file in target_dir.rglob("*.md"):
        modified, blocks = process_markdown_file(md_file)
        total_files += 1
        total_modified_blocks += modified
        total_blocks += blocks

        if modified > 0:
            rel_path = md_file.relative_to(target_dir)
            print(f"  ✅ {rel_path}: 修改 {modified}/{blocks} 个代码块")

    print()
    print(f"📊 统计:")
    print(f"  - 处理文件: {total_files}")
    print(f"  - 总代码块: {total_blocks}")
    print(f"  - 修改代码块: {total_modified_blocks}")


if __name__ == '__main__':
    main()


if __name__ == "__main__":
    main()
