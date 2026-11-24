#pragma once
#include <stddef.h>
#ifdef __cplusplus
extern "C" {
#endif
void *malloc(size_t n);
void free(void *p);
void *calloc(size_t a, size_t b);
void *realloc(void*p,size_t n);
void abort(void);
#ifdef __cplusplus
}
#endif
static inline void exit(int c){__builtin_trap();}
