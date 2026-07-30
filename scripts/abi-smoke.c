#include <dlfcn.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

typedef struct {
    void *ptr;
    size_t len;
} cliproxy_buffer;

typedef int (*cliproxy_host_call_fn)(void *, const char *, const uint8_t *, size_t, cliproxy_buffer *);
typedef void (*cliproxy_host_free_fn)(void *, size_t);

typedef struct {
    uint32_t abi_version;
    void *host_ctx;
    cliproxy_host_call_fn call;
    cliproxy_host_free_fn free_buffer;
} cliproxy_host_api;

typedef int (*cliproxy_plugin_call_fn)(const char *, const uint8_t *, size_t, cliproxy_buffer *);
typedef void (*cliproxy_plugin_free_fn)(void *, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);

typedef struct {
    uint32_t abi_version;
    cliproxy_plugin_call_fn call;
    cliproxy_plugin_free_fn free_buffer;
    cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;

typedef int (*cliproxy_plugin_init_fn)(const cliproxy_host_api *, cliproxy_plugin_api *);

int main(int argc, char **argv) {
    if (argc != 2 && argc != 3) {
        fprintf(stderr, "usage: %s <plugin-library> [expected-default-database]\n", argv[0]);
        return 2;
    }

    const char *expected_default_database = argc == 3 ? argv[2] : NULL;

    void *library = dlopen(argv[1], RTLD_NOW | RTLD_LOCAL);
    if (library == NULL) {
        fprintf(stderr, "dlopen failed: %s\n", dlerror());
        return 3;
    }

    dlerror();
    cliproxy_plugin_init_fn init = (cliproxy_plugin_init_fn)dlsym(library, "cliproxy_plugin_init");
    const char *symbol_error = dlerror();
    if (symbol_error != NULL || init == NULL) {
        fprintf(stderr, "dlsym failed: %s\n", symbol_error == NULL ? "missing symbol" : symbol_error);
        dlclose(library);
        return 4;
    }

    cliproxy_host_api host = {.abi_version = 1};
    cliproxy_plugin_api plugin = {0};
    if (init(&host, &plugin) != 0 || plugin.abi_version != 1 || plugin.call == NULL || plugin.free_buffer == NULL || plugin.shutdown == NULL) {
        fprintf(stderr, "plugin initialization returned an invalid function table\n");
        dlclose(library);
        return 5;
    }

    const char explicit_path_request[] = "{\"config_yaml\":\"ZGF0YV9wYXRoOiAvdG1wL2NhcC10b2tlbi11c2FnZS1zbW9rZS5kYgo=\",\"schema_version\":2}";
    const char default_path_request[] = "{\"schema_version\":2}";
    const char *request = expected_default_database == NULL ? explicit_path_request : default_path_request;
    cliproxy_buffer response = {0};
    if (plugin.call("plugin.register", (const uint8_t *)request, strlen(request), &response) != 0 || response.ptr == NULL || response.len == 0) {
        fprintf(stderr, "plugin.register transport call failed\n");
        plugin.shutdown();
        dlclose(library);
        return 6;
    }

    char *json = malloc(response.len + 1);
    if (json == NULL) {
        plugin.free_buffer(response.ptr, response.len);
        plugin.shutdown();
        dlclose(library);
        return 7;
    }
    memcpy(json, response.ptr, response.len);
    json[response.len] = '\0';
    plugin.free_buffer(response.ptr, response.len);

    if (strstr(json, "\"ok\":true") == NULL || strstr(json, "\"schema_version\":2") == NULL || strstr(json, "\"usage_plugin\":true") == NULL || strstr(json, "\"management_api\":true") == NULL) {
        fprintf(stderr, "unexpected registration response: %s\n", json);
        free(json);
        plugin.shutdown();
        dlclose(library);
        return 8;
    }

    free(json);
    plugin.shutdown();
    dlclose(library);

    if (expected_default_database != NULL) {
        if (access(expected_default_database, F_OK) != 0) {
            fprintf(stderr, "default database was not created at %s\n", expected_default_database);
            return 9;
        }
        unlink(expected_default_database);
    } else {
        unlink("/tmp/cap-token-usage-smoke.db");
    }
    printf("ABI smoke test passed\n");
    return 0;
}
