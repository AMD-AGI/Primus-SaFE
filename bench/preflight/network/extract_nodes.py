#!/usr/bin/env python3
import re
import ast
import sys

def extract_unhealthy_nodes(log_line):
    match = re.search(r"unhealthy nodes:\s*(\[[^\]]*\])", log_line)
    if match:
        nodes_str = match.group(1)
        try:
            nodes_list = ast.literal_eval(nodes_str)
            return ','.join(nodes_list)
        except:
            return ''
    else:
        return ''

if __name__ == "__main__":
    if len(sys.argv) > 1:
        result = extract_unhealthy_nodes(sys.argv[1])
        print(result)