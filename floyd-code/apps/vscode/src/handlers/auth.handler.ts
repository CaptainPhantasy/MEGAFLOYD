import { Methods } from "../../shared/bridge";
import type { LoginStatus } from "../../shared/types";
import { updateLoginContext } from "../utils/context";
import type { Handler } from "./types";

export const authHandlers: Record<string, Handler<any, any>> = {
  [Methods.CheckLoginStatus]: async (_, ctx): Promise<LoginStatus> => {
    return { loggedIn: await updateLoginContext(ctx.harness) };
  },
};
