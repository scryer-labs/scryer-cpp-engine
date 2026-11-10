#ifndef SCRYER_ENGINE_V1_H
#define SCRYER_ENGINE_V1_H
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

typedef struct {
    const char *data;
    size_t size;
} scryer_view_v1;

typedef struct {
    char *data;
    size_t size;
} scryer_buffer_v1;

typedef struct {
    int32_t status;
    scryer_buffer_v1 response;
} scryer_result_v1;

enum {
    SCRYER_OK_V1 = 0,
    SCRYER_ERROR_V1 = 1,
    SCRYER_CANCELLED_V1 = 2,
    SCRYER_BUSY_V1 = 3
};

#ifdef __cplusplus
extern "C" {
#endif
scryer_result_v1 execute_v1(scryer_view_v1 request);
bool cancel_v1(scryer_view_v1 request_id);
void release_buffer_v1(scryer_buffer_v1 buffer);
#ifdef __cplusplus
}
#endif
#endif
