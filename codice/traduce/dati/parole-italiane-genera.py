# Genera dati/parole-italiane.txt.gz: elenco di parole italiane (minuscole, forme base) per il
# controllo delle traduzioni del traduttore veloce (italiano.go riconosce da qui verbi coniugati,
# plurali, avverbi, apostrofi e pronomi attaccati). Fonti: le frasi italiane già nel programma
# (frasi pronte, glossario, libreria dei giochi liberi, frasi di prova), i testi e i commenti
# italiani del codice (parole viste almeno 2 volte), un elenco di parole comuni scritto a mano
# (parole-italiane-comuni.txt). Le parole in parole-italiane-no.txt (traduzioni sbagliate citate
# nei commenti) restano fuori.
import re, sys, gzip, json, collections, os

# Uso: python3 parole-italiane-genera.py <cartella codice/traduce> <cartella radice del progetto>
# Scrive dati/parole-italiane.txt.gz accanto a questo file.
R = sys.argv[1]
RADICE = sys.argv[2]
DATI = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.join(DATI, 'parole-italiane.txt.gz')

LET = r"[a-zàèéìíòóùú]+"
def parole(t):
    return re.findall(LET, t.lower())

dati = collections.Counter()
with gzip.open(os.path.join(R, 'dati/libreria-giochi-liberi.tsv.gz'), 'rt', encoding='utf-8') as f:
    for l in f:
        if l.startswith('#') or '\t' not in l: continue
        dati.update(parole(l.split('\t', 1)[1]))
for nome in ['dati/frasi-menu.tsv', 'dati/glossario-giochi.tsv']:
    for l in open(os.path.join(R, nome), encoding='utf-8'):
        if l.startswith('#') or '\t' not in l: continue
        dati.update(parole(l.split('\t', 1)[1]))
for nome in ['prova/test_1000.jsonl', 'prova/test_2000_extra.jsonl']:
    for l in open(os.path.join(R, nome), encoding='utf-8'):
        dati.update(parole(json.loads(l)['it']))
# testi e commenti italiani del codice (non i test, non le librerie di altri, non le altre lingue)
repo = collections.Counter()
for cart, sotto, file in os.walk(RADICE):
    sotto[:] = [d for d in sotto if d not in ('.git', 'third_party', 'node_modules', 'dist')]
    for f in file:
        if not f.endswith(('.go', '.md', '.html', '.py')) or f.endswith('_test.go'):
            continue
        repo.update(parole(open(os.path.join(cart, f), encoding='utf-8', errors='ignore').read()))

comuni = set()
for l in open(os.path.join(DATI, 'parole-italiane-comuni.txt'), encoding='utf-8'):
    l = l.split('#', 1)[0]
    comuni.update(parole(l))

base = set(w for w, n in dati.items() if n >= 1 and len(w) >= 1)
base |= set(w for w, n in repo.items() if n >= 2)
base |= comuni
# parole tolte a mano (errori o parole inglesi/codice che non sono italiane)
NO = set(w for l in open(os.path.join(DATI, 'parole-italiane-no.txt'), encoding='utf-8') if not l.startswith('#') for w in parole(l))

parole_tutte = set(base)

parole_tutte -= NO
parole_tutte = sorted(w for w in parole_tutte if len(w) >= 1)
with gzip.open(OUT, 'wt', encoding='utf-8', compresslevel=9) as f:
    f.write('# Parole italiane per il controllo delle traduzioni (Tasso Traduce, GPL-3.0). Generato da parole-italiane-genera.py.\n')
    for w in parole_tutte:
        f.write(w + '\n')
print(len(parole_tutte), os.path.getsize(OUT))
