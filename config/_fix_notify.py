import re, glob

# service/notify: every signature takes *config.Config -> *config.RuntimeSettings
for path in glob.glob('service/notify/*.go'):
    if path.endswith('_test.go'):
        continue
    src = open(path, encoding='utf-8').read()
    src = src.replace('cfg *config.Config', 'cfg *config.RuntimeSettings')
    open(path, 'w', encoding='utf-8').write(src)
print("notify signatures updated")
