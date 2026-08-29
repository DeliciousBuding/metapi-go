import glob
for path in ['service/balance/balance.go', 'service/checkin/checkin.go']:
    src = open(path, encoding='utf-8').read()
    src = src.replace('cfg *config.Config', 'cfg *config.RuntimeSettings')
    open(path, 'w', encoding='utf-8').write(src)
print("balance+checkin updated")
