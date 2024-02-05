import { uuidv7 } from "./uuidv7"
/** per-instance unique identifier */
const INSTANCE_ID = uuidv7()
const load_env = () => {
    const env = process.env.NODE_ENV || "development"
    const is_dev = env === "development"
    return { env, is_dev }  

}