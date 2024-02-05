
/**  log a message and optional fields at the specified level.
see the repository-level documentation of rplog for more details on logging.
all logs are JSON-formatted.
 * @param msg - the message to log
 * @param level - the log level to use
 * @param fields - optional fields to include in the log message
 * @param include_metadata - whether to include metadata in the log message
 * @param logfn - the function to use to log the message (defaults to console.log, which writes to stdout)
 * @param stack_depth - the number of stack frames to include in the log message. if 0, no stack trace is included. if -1, the entire stack trace is included. if a positive number, that many stack frames are included.
*/
export const log = (msg: string, level="info", fields ={}, include_metadata=true, logfn=console.log, stack_depth=0) => {
    if (typeof(level) !== 'string') {
        level = 'info'
    }
    if (stack_depth !== 0) {
        let stack = (new Error()).stack?.split("\n").map((x) => x.trim()).filter((x) => x.length > 0) || []
        if (stack_depth > stack.length || stack_depth === -1){
            stack_depth = stack.length
        }
        stack = stack.slice(1, stack_depth+1)
        fields["stack"] = stack
    }
    // TODO: add metadata
    fields["level"] = level
    fields["msg"] = msg
    fields["time"] = (new Date()).toISOString()
    logfn(JSON.stringify( fields))
}

var metacache = {}





/**  log a message and optional fields at the debug level. use the log fn directly if you want finer control over the log message*/
export const debug = (msg: string, fields={}) => log(msg, 'debug', fields, true, console.log, 0)
/**  log a message and optional fields at the info level. use the log fn directly if you want finer control over the log message*/
export const info = (msg: string, fields={}) => log(msg, 'info', fields, true, console.log, 0)
/**  log a message and optional fields at the warn level. use the log fn directly if you want finer control over the log message*/
export const warn = (msg: string, fields={}) => log(msg, 'warn', fields, true, console.log, 0)
/**  log a message and optional fields at the error level. use the log fn directly if you want finer control over the log message*/
export const error = (msg: string, fields={}) => log(msg, 'error', fields, true, console.log, -1)
