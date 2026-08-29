src = open('handler/proxy/responses_ws.go', encoding='utf-8').read()
old = '\t"github.com/deliciousbuding/metapi-go/auth"\n'
new = '\t"github.com/deliciousbuding/metapi-go/auth"\n\t"github.com/deliciousbuding/metapi-go/config"\n'
assert old in src
src = src.replace(old, new, 1)
open('handler/proxy/responses_ws.go','w',encoding='utf-8').write(src)
print("import added")
