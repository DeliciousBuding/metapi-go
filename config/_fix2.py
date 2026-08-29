import re
src = open('config/config.go', encoding='utf-8').read()
src2 = re.sub(r'\t"sync"\r?\n', '', src, count=1)
assert src2 != src
open('config/config.go','w',encoding='utf-8').write(src2)
print("ok")
