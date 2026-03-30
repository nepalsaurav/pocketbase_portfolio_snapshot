<script>
    import TopNav from "./components/TopNav.svelte";
    import { routes } from "./routes";
    import Router from "svelte-spa-router";
</script>


<TopNav/>
<div class="container mt-4">
    <Router {routes} />
</div>
