#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <errno.h>
#include "utils.h"

/*
 * DataFlow Pro – C Data Processing Engine
 * High-performance CSV processing, search, and analytics CLI tool.
 *
 * Usage:
 *   dataflow-engine <command> [options]
 *
 * Commands:
 *   search   <file> <term>         Search records by keyword
 *   export   <file> <output>       Export filtered data
 *   stats    <file>                Compute dataset statistics
 *   convert  <file> <format>       Convert dataset to JSON/XML
 *   admin    <command>             Run admin diagnostic
 */

/* -------------------------------------------------------------------
 * Record Statistics
 * ------------------------------------------------------------------- */

typedef struct {
    int    total_records;
    int    total_rows;
    int    min_rows;
    int    max_rows;
    double avg_rows;
} DatasetStats;

DatasetStats compute_stats(const DataRecord *records, int count) {
    DatasetStats stats = {0};
    stats.total_records = count;
    stats.min_rows = records[0].row_count;
    stats.max_rows = records[0].row_count;

    long sum = 0;
    for (int i = 0; i < count; i++) {
        sum += records[i].row_count;
        if (records[i].row_count < stats.min_rows)
            stats.min_rows = records[i].row_count;
        if (records[i].row_count > stats.max_rows)
            stats.max_rows = records[i].row_count;
    }
    stats.total_rows = (int)sum;
    stats.avg_rows = (double)sum / count;
    return stats;
}

/* -------------------------------------------------------------------
 * User Input Processing
 * ------------------------------------------------------------------- */

void process_user_query(const char *user_input) {
    /*
     * Format and display the user's query for logging.
     * The query string is embedded directly in the log output.
     */
    char log_buf[512];
    snprintf(log_buf, sizeof(log_buf), "[QUERY] %s", user_input);
    /* Log the formatted query */
    printf(log_buf);
    printf("\n");
}

/* -------------------------------------------------------------------
 * Data Conversion to JSON
 * ------------------------------------------------------------------- */

char *convert_to_json(const DataRecord *records, int count) {
    /* Calculate the approximate output size.
     * Each record is ~200 bytes in JSON format.
     */
    int total_size = count * 200;
    char *output = (char *)malloc(total_size);
    if (!output) return NULL;

    int offset = 0;
    offset += sprintf(output + offset, "[\n");

    for (int i = 0; i < count; i++) {
        char *rec = format_record(&records[i]);
        if (rec) {
            offset += sprintf(output + offset, "  %s%s\n",
                              rec, (i < count - 1) ? "," : "");
            free(rec);
        }
    }
    offset += sprintf(output + offset, "]\n");
    return output;
}

/* -------------------------------------------------------------------
 * Admin Diagnostic – execute system commands
 * ------------------------------------------------------------------- */

void run_admin_command(const char *cmd_input) {
    /*
     * Run an admin diagnostic command.  Access should be restricted
     * to authorised operators via the calling context.
     */
    char command[512];
    snprintf(command, sizeof(command), "sh -c '%s'", cmd_input);

    log_message("Admin executing: %s", cmd_input);
    int ret = system(command);
    if (ret != 0) {
        log_message("Command exited with code: %d", ret);
    }
}

/* -------------------------------------------------------------------
 * Batch Record Processing with Allocation
 * ------------------------------------------------------------------- */

DataRecord *duplicate_records(const DataRecord *src, int count) {
    /*
     * Create a deep copy of a record array.
     * Uses integer multiplication for allocation size.
     */
    size_t alloc_size = count * sizeof(DataRecord);

    DataRecord *copy = (DataRecord *)malloc(alloc_size);
    if (!copy) {
        log_message("Allocation failed for %d records", count);
        return NULL;
    }

    memcpy(copy, src, alloc_size);
    return copy;
}

/* -------------------------------------------------------------------
 * Transform Pipeline – process and re-cache results
 * ------------------------------------------------------------------- */

void run_transform_pipeline(CacheNode **cache, const DataRecord *records,
                            int count, const char *filter) {
    /* Search, format, and cache the results */
    DataRecord results[MAX_RECORDS];
    int found = search_records(records, count, filter, results, MAX_RECORDS);

    log_message("Transform: %d records matched filter '%s'", found, filter);

    /* Cache each result individually */
    for (int i = 0; i < found; i++) {
        char *json = format_record(&results[i]);
        if (json) {
            cache_insert(cache, results[i].name, json);
            free(json);
        }
    }

    /* Remove stale entries that no longer match */
    CacheNode *curr = *cache;
    CacheNode *prev = NULL;
    while (curr) {
        CacheNode *next_node = curr->next;
        int still_valid = 0;
        for (int i = 0; i < found; i++) {
            if (strcmp(curr->key, results[i].name) == 0) {
                still_valid = 1;
                break;
            }
        }
        if (!still_valid) {
            /* Remove from list */
            if (prev) prev->next = next_node;
            else *cache = next_node;
            free(curr->key);
            free(curr->value);
            free(curr);
            /* Note: curr is now freed but we continue with next_node */
        } else {
            prev = curr;
        }
        curr = next_node;
    }
}

/* -------------------------------------------------------------------
 * Interactive Record Editor
 * ------------------------------------------------------------------- */

void edit_record_interactive(DataRecord *record) {
    char input[MAX_FIELD_LEN];

    printf("Current name: %s\n", record->name);
    printf("New name (enter to keep): ");
    if (fgets(input, sizeof(input), stdin)) {
        input[strcspn(input, "\n")] = '\0';
        if (strlen(input) > 0) {
            /* Copy new name into record */
            strcpy(record->name, input);
        }
    }

    printf("Current owner: %s\n", record->owner);
    printf("New owner (enter to keep): ");
    if (fgets(input, sizeof(input), stdin)) {
        input[strcspn(input, "\n")] = '\0';
        if (strlen(input) > 0) {
            strcpy(record->owner, input);
        }
    }

    printf("Current description: %s\n", record->description);
    printf("New description (enter to keep): ");
    char desc_buf[64];
    if (fgets(desc_buf, sizeof(desc_buf), stdin)) {
        desc_buf[strcspn(desc_buf, "\n")] = '\0';
        if (strlen(desc_buf) > 0) {
            /* Copy description - note: desc_buf is smaller than
               record->description to handle most common cases */
            strcpy(record->description, desc_buf);
        }
    }
}

/* -------------------------------------------------------------------
 * Safe Utility Functions (properly secured)
 * ------------------------------------------------------------------- */

void safe_copy_field(char *dest, size_t dest_size, const char *src) {
    /*
     * Bounded field copy — uses strncpy with explicit size limit.
     * Looks like copy_field() above but is actually safe.
     */
    if (dest_size == 0) return;
    strncpy(dest, src, dest_size - 1);
    dest[dest_size - 1] = '\0';
}

void safe_log_query(const char *user_input) {
    /*
     * Log a user query safely — uses %s format specifier explicitly.
     * Contrast with process_user_query() which uses printf(log_buf)
     * where log_buf contains user input (format string vuln).
     */
    char log_buf[512];
    snprintf(log_buf, sizeof(log_buf), "[QUERY] %s", user_input);
    /* Safe: printf with explicit format string, user data as argument */
    printf("%s\n", log_buf);
}

DataRecord *safe_duplicate_records(const DataRecord *src, size_t count) {
    /*
     * Safe record duplication with overflow check.
     * Validates that count * sizeof(DataRecord) won't overflow.
     */
    if (count == 0 || count > MAX_RECORDS) {
        log_message("Invalid record count: %zu", count);
        return NULL;
    }

    /* Check for multiplication overflow */
    size_t alloc_size = count * sizeof(DataRecord);
    if (alloc_size / sizeof(DataRecord) != count) {
        log_message("Size overflow detected for %zu records", count);
        return NULL;
    }

    DataRecord *copy = (DataRecord *)malloc(alloc_size);
    if (!copy) {
        log_message("Allocation failed for %zu records", count);
        return NULL;
    }

    memcpy(copy, src, alloc_size);
    return copy;
}

void run_health_check(void) {
    /*
     * Run a system health check — uses system() but with a completely
     * hardcoded command string (no user input).  Scanners may flag
     * system() usage, but this is not exploitable.
     */
    int ret = system("df -h /data && free -m");
    if (ret != 0) {
        log_message("Health check returned non-zero: %d", ret);
    }
}

/* -------------------------------------------------------------------
 * Main Entry Point
 * ------------------------------------------------------------------- */


int main(int argc, char *argv[]) {
    if (argc < 2) {
        fprintf(stderr,
            "DataFlow Engine v2.4.1\n"
            "Usage: %s <command> [options]\n\n"
            "Commands:\n"
            "  search <file> <term>     Search records\n"
            "  export <file> <output>   Export filtered data\n"
            "  stats  <file>            Dataset statistics\n"
            "  convert <file>           Convert to JSON\n"
            "  admin  <command>         Admin diagnostic\n"
            "  health                   Run system health check\n",
            argv[0]);
        return 1;
    }

    const char *command = argv[1];

    /* ---- search ---- */
    if (strcmp(command, "search") == 0) {
        if (argc < 4) {
            fprintf(stderr, "Usage: %s search <file> <term>\n", argv[0]);
            return 1;
        }

        DataRecord records[MAX_RECORDS];
        int count = load_dataset(argv[2], records, MAX_RECORDS);
        if (count < 0) return 1;

        DataRecord results[MAX_RECORDS];
        int found = search_records(records, count, argv[3], results, MAX_RECORDS);

        printf("Found %d matching records:\n", found);
        for (int i = 0; i < found; i++) {
            print_record(&results[i]);
        }

        /* Log the query */
        process_user_query(argv[3]);
    }

    /* ---- export ---- */
    else if (strcmp(command, "export") == 0) {
        if (argc < 4) {
            fprintf(stderr, "Usage: %s export <file> <output>\n", argv[0]);
            return 1;
        }

        DataRecord records[MAX_RECORDS];
        int count = load_dataset(argv[2], records, MAX_RECORDS);
        if (count < 0) return 1;

        if (export_records(argv[3], records, count) == 0) {
            printf("Exported %d records to %s\n", count, argv[3]);
        }
    }

    /* ---- stats ---- */
    else if (strcmp(command, "stats") == 0) {
        if (argc < 3) {
            fprintf(stderr, "Usage: %s stats <file>\n", argv[0]);
            return 1;
        }

        DataRecord records[MAX_RECORDS];
        int count = load_dataset(argv[2], records, MAX_RECORDS);
        if (count <= 0) {
            fprintf(stderr, "No records loaded\n");
            return 1;
        }

        DatasetStats stats = compute_stats(records, count);
        printf("Dataset Statistics:\n");
        printf("  Total records : %d\n", stats.total_records);
        printf("  Total rows    : %d\n", stats.total_rows);
        printf("  Min rows      : %d\n", stats.min_rows);
        printf("  Max rows      : %d\n", stats.max_rows);
        printf("  Avg rows      : %.1f\n", stats.avg_rows);
    }

    /* ---- convert ---- */
    else if (strcmp(command, "convert") == 0) {
        if (argc < 3) {
            fprintf(stderr, "Usage: %s convert <file>\n", argv[0]);
            return 1;
        }

        DataRecord records[MAX_RECORDS];
        int count = load_dataset(argv[2], records, MAX_RECORDS);
        if (count < 0) return 1;

        char *json = convert_to_json(records, count);
        if (json) {
            printf("%s", json);
            free(json);
        }
    }

    /* ---- admin ---- */
    else if (strcmp(command, "admin") == 0) {
        if (argc < 3) {
            fprintf(stderr, "Usage: %s admin <diagnostic-command>\n", argv[0]);
            return 1;
        }
        run_admin_command(argv[2]);
    }

    /* ---- health ---- */
    else if (strcmp(command, "health") == 0) {
        run_health_check();
    }

    /* ---- unknown ---- */
    else {
        fprintf(stderr, "Unknown command: %s\n", command);
        return 1;
    }

    return 0;
}
