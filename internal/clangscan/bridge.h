#ifndef SCRYER_CLANG_BRIDGE_H
#define SCRYER_CLANG_BRIDGE_H
#include <stdint.h>

typedef struct ScryerClang ScryerClang;
ScryerClang *scryer_clang_open(const char *path);
void scryer_clang_close(ScryerClang *api);
int scryer_clang_scan(ScryerClang *api, const char *const *args,
                      int argc, uintptr_t handle, int includes, int calls);
#endif
