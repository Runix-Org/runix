#pragma once

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

void *runix_capture_new(uintptr_t handle);
// Returns false unless a focused native Wayland surface and inhibitor are available.
int runix_capture_start(void *capture);
void runix_capture_stop(void *capture);
void runix_capture_delete(void *capture);

#ifdef __cplusplus
}
#endif
