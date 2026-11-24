#include <stddef.h>
extern unsigned char __heap_base;
static unsigned char *top = 0;
static size_t pages_bytes(void){ return __builtin_wasm_memory_size(0) * 65536; }
/* semplice: alloca a salire, libera tutto con oz_reset */
void *malloc(size_t n){
  if(!top) top = &__heap_base;
  size_t a = ((size_t)top + 15) & ~(size_t)15;
  size_t end = a + n;
  while (end > pages_bytes()) { if (__builtin_wasm_memory_grow(0, (end - pages_bytes() + 65535)/65536) == (size_t)-1) return 0; }
  top = (unsigned char*)end; return (void*)a;
}
void free(void *p){ (void)p; }
void *calloc(size_t a, size_t b){ unsigned char*p=malloc(a*b); if(p) __builtin_memset(p,0,a*b); return p; }
void *realloc(void*p,size_t n){ return 0; }
void abort(void){ __builtin_trap(); }
void *memcpy(void *d, const void *s, size_t n){ unsigned char*dd=d; const unsigned char*ss=s; while(n--) *dd++=*ss++; return d; }
void *memmove(void *d, const void *s, size_t n){ unsigned char*dd=d; const unsigned char*ss=s; if(dd<ss){while(n--)*dd++=*ss++;} else {dd+=n;ss+=n;while(n--)*--dd=*--ss;} return d; }
void *memset(void *d, int c, size_t n){ unsigned char*dd=d; while(n--)*dd++=(unsigned char)c; return d; }
int memcmp(const void *a, const void *b, size_t n){ const unsigned char*x=a,*y=b; for(;n;n--,x++,y++) if(*x!=*y) return *x-*y; return 0; }
size_t strlen(const char*s){ size_t n=0; while(s[n]) n++; return n; }
int Ooz_Decompress(const unsigned char*, int, unsigned char*, size_t, int,int,int, unsigned char*, size_t, void*,void*,void*, size_t, int);
/* API per Tasso: oz_reset, oz_alloc, oz_dec */
void oz_reset(void){ top = &__heap_base; }
void *oz_alloc(size_t n){ return malloc(n); }
int oz_dec(const unsigned char *src, int src_len, unsigned char *dst, int dst_len){ return Ooz_Decompress(src, src_len, dst, (size_t)dst_len, 0,0,0,0,0,0,0,0,0,0); }
