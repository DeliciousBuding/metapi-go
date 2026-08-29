import re, glob
for path in glob.glob('**/*.go', recursive=True):
    if 'web' in path.split('\\')[:1]:
        continue
    try:
        src = open(path, encoding='utf-8').read()
    except (UnicodeDecodeError, OSError):
        continue
    new = re.sub(r'NewProxyChannelCoordinator\([^)]*\)', 'NewProxyChannelCoordinator()', src)
    if new != src:
        open(path, 'w', encoding='utf-8').write(new)
        print("fixed:", path)
