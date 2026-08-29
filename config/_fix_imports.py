import re
src = open('config/config.go', encoding='utf-8').read()
m = re.search(r'import \((.*?)\n\)', src, re.S)
block = m.group(1)
block = block.replace('\t"sync"\n', '')
if '"sync/atomic"' not in block:
    block = block.replace('\t"strconv"\n', '\t"strconv"\n\t"sync/atomic"\n')
src = src[:m.start(1)] + block + src[m.end(1):]
open('config/config.go','w',encoding='utf-8').write(src)
print("imports fixed")
