#include "bridge.h"
#include <dlfcn.h>
#include <stdlib.h>

// Public libclang C ABI types, kept here so static scans need no LLVM headers.
typedef void *CXIndex;
typedef struct CXTranslationUnitImpl *CXTranslationUnit;
typedef void *CXFile;
typedef void *CXDiagnostic;
typedef struct CXTargetInfoImpl *CXTargetInfo;
typedef struct { const void *data; unsigned private_flags; } CXString;
typedef struct { const void *ptr_data[2]; unsigned int_data; } CXSourceLocation;
typedef struct { unsigned kind; int xdata; const void *data[3]; } CXCursor;
typedef unsigned (*CXCursorVisitor)(CXCursor, CXCursor, void *);

struct ScryerClang {
    void *library;
    CXIndex (*createIndex)(int, int);
    void (*disposeIndex)(CXIndex);
    int (*parse)(CXIndex, const char *, const char *const *, int, void *, unsigned, unsigned, CXTranslationUnit *);
    void (*disposeTU)(CXTranslationUnit);
    CXTargetInfo (*targetInfo)(CXTranslationUnit);
    CXString (*targetTriple)(CXTargetInfo);
    void (*disposeTarget)(CXTargetInfo);
    unsigned (*numDiagnostics)(CXTranslationUnit);
    CXDiagnostic (*diagnostic)(CXTranslationUnit, unsigned);
    unsigned (*severity)(CXDiagnostic);
    CXString (*diagnosticSpelling)(CXDiagnostic);
    void (*disposeDiagnostic)(CXDiagnostic);
    CXCursor (*root)(CXTranslationUnit);
    unsigned (*visit)(CXCursor, CXCursorVisitor, void *);
    CXCursor (*referenced)(CXCursor);
    CXCursor (*definition)(CXCursor);
    CXSourceLocation (*cursorLocation)(CXCursor);
    void (*fileLocation)(CXSourceLocation, CXFile *, unsigned *, unsigned *, unsigned *);
    CXFile (*includedFile)(CXCursor);
    CXString (*fileName)(CXFile);
    CXString (*spelling)(CXCursor);
    CXString (*usr)(CXCursor);
    const char *(*cstring)(CXString);
    void (*disposeString)(CXString);
};

#define LOAD(field, symbol) do { api->field = dlsym(api->library, symbol); if (!api->field) goto failure; } while (0)
ScryerClang *scryer_clang_open(const char *path) {
    ScryerClang *api = calloc(1, sizeof(*api));
    if (!api) return NULL;
    api->library = dlopen(path, RTLD_NOW | RTLD_LOCAL);
    if (!api->library) goto failure;
    LOAD(createIndex, "clang_createIndex");
    LOAD(disposeIndex, "clang_disposeIndex");
    LOAD(parse, "clang_parseTranslationUnit2FullArgv");
    LOAD(disposeTU, "clang_disposeTranslationUnit");
    LOAD(targetInfo, "clang_getTranslationUnitTargetInfo");
    LOAD(targetTriple, "clang_TargetInfo_getTriple");
    LOAD(disposeTarget, "clang_TargetInfo_dispose");
    LOAD(numDiagnostics, "clang_getNumDiagnostics");
    LOAD(diagnostic, "clang_getDiagnostic");
    LOAD(severity, "clang_getDiagnosticSeverity");
    LOAD(diagnosticSpelling, "clang_getDiagnosticSpelling");
    LOAD(disposeDiagnostic, "clang_disposeDiagnostic");
    LOAD(root, "clang_getTranslationUnitCursor");
    LOAD(visit, "clang_visitChildren");
    LOAD(referenced, "clang_getCursorReferenced");
    LOAD(definition, "clang_getCursorDefinition");
    LOAD(cursorLocation, "clang_getCursorLocation");
    LOAD(fileLocation, "clang_getFileLocation");
    LOAD(includedFile, "clang_getIncludedFile");
    LOAD(fileName, "clang_getFileName");
    LOAD(spelling, "clang_getCursorSpelling");
    LOAD(usr, "clang_getCursorUSR");
    LOAD(cstring, "clang_getCString");
    LOAD(disposeString, "clang_disposeString");
    return api;
failure:
    scryer_clang_close(api);
    return NULL;
}
#undef LOAD

void scryer_clang_close(ScryerClang *api) {
    if (api->library) dlclose(api->library);
    free(api);
}

extern int scryerClangCancelled(uintptr_t handle);
extern int scryerClangLocal(uintptr_t handle, char *file);
extern void scryerClangTarget(uintptr_t handle, char *triple);
extern void scryerClangDiagnostic(uintptr_t handle, unsigned severity, char *message);
extern void scryerClangEdge(uintptr_t handle, unsigned kind, char *file,
                            unsigned line, unsigned column, char *target_file,
                            unsigned target_line, unsigned target_column, int definition,
                            char *name, char *usr);

typedef struct {
    ScryerClang *api;
    uintptr_t handle;
    int includes;
    int calls;
} Visitor;

static unsigned visit_cursor(CXCursor cursor, CXCursor parent, void *opaque) {
    Visitor *visitor = opaque;
    ScryerClang *api = visitor->api;
    if (scryerClangCancelled(visitor->handle)) return 0;
    const unsigned call = 103, inclusion = 503;
    CXFile source = NULL;
    unsigned line = 0, column = 0;
    api->fileLocation(api->cursorLocation(cursor), &source, &line, &column, NULL);
    if (!source) return 1;
    CXString source_name = api->fileName(source);
    const char *file = api->cstring(source_name);
    if (!scryerClangLocal(visitor->handle, (char *)file)) {
        api->disposeString(source_name);
        return 1;
    }
    if ((cursor.kind == call && visitor->calls) || (cursor.kind == inclusion && visitor->includes)) {
        CXFile target = NULL;
        unsigned target_line = 0, target_column = 0;
        int is_definition = 0;
        CXCursor ref = api->referenced(cursor);
        if (cursor.kind == inclusion) {
            target = api->includedFile(cursor);
        } else {
            CXCursor definition = api->definition(ref);
            api->fileLocation(api->cursorLocation(definition), &target, &target_line, &target_column, NULL);
            is_definition = target != NULL;
            if (!target)
                api->fileLocation(api->cursorLocation(ref), &target, &target_line, &target_column, NULL);
        }
        if (target) {
            CXString target_name = api->fileName(target);
            CXString name = api->spelling(cursor.kind == call ? ref : cursor);
            CXString usr = api->usr(ref);
            scryerClangEdge(visitor->handle, cursor.kind, (char *)file, line, column,
                            (char *)api->cstring(target_name), target_line, target_column, is_definition,
                            (char *)api->cstring(name), (char *)api->cstring(usr));
            api->disposeString(usr);
            api->disposeString(name);
            api->disposeString(target_name);
        }
    }
    api->disposeString(source_name);
    return 2;
}

int scryer_clang_scan(ScryerClang *api, const char *const *args,
                      int argc, uintptr_t handle, int includes, int calls) {
    CXIndex index = api->createIndex(0, 0);
    if (!index) return 1;
    CXTranslationUnit unit = NULL;
    int status = api->parse(index, NULL, args, argc, NULL, 0, includes ? 1 : 0, &unit);
    if (status == 0 && unit) {
        CXTargetInfo target = api->targetInfo(unit);
        if (target) {
            CXString triple = api->targetTriple(target);
            scryerClangTarget(handle, (char *)api->cstring(triple));
            api->disposeString(triple);
            api->disposeTarget(target);
        }
        unsigned count = api->numDiagnostics(unit);
        for (unsigned i = 0; i < count; i++) {
            CXDiagnostic diagnostic = api->diagnostic(unit, i);
            CXString message = api->diagnosticSpelling(diagnostic);
            scryerClangDiagnostic(handle, api->severity(diagnostic), (char *)api->cstring(message));
            api->disposeString(message);
            api->disposeDiagnostic(diagnostic);
        }
        Visitor visitor = {api, handle, includes, calls};
        api->visit(api->root(unit), visit_cursor, &visitor);
    }
    if (unit) api->disposeTU(unit);
    api->disposeIndex(index);
    return status;
}
