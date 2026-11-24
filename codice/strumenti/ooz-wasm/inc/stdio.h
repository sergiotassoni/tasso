#pragma once
#include <stddef.h>
typedef struct FILE FILE;
#define stderr ((FILE*)0)
static inline int fprintf(FILE*f,const char*s,...){return 0;}
static inline int printf(const char*s,...){return 0;}
