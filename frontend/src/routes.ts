import Home from "./routes/Home.svelte";
import NotFound from "./routes/NotFound.svelte"
import ImportTransactions from "./routes/ImportTransactions.svelte"

export const routes = {
  "/": Home,
  "/import_transactions": ImportTransactions,
  "*": NotFound
};
