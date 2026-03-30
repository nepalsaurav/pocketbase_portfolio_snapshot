<script lang="ts">
    import { link } from "svelte-spa-router";
    interface NavItemProps {
        url: string;
        label: string;
        icon: string; // Added icon prop
        active?: boolean;
    }
</script>

{#snippet NavItem({ url, label, icon, active }: NavItemProps)}
    <li class="nav-item">
        <a
            class="nav-link d-flex align-items-center {active
                ? 'active fw-bold'
                : ''}"
            aria-current={active ? "page" : undefined}
            href={url}
            use:link
        >
            <i class="bi {icon} me-2"></i>
            <span>{label}</span>
        </a>
    </li>
{/snippet}

<nav class="navbar navbar-expand-lg navbar-dark bg-dark py-3 shadow">
    <div class="container">
        <a
            class="navbar-brand fw-bold text-uppercase tracking-tight d-flex align-items-center"
            href="/"
        >
            <i class="bi bi-pie-chart-fill me-2 text-primary"></i>
            <span>Portfolio Snapshot</span>
        </a>

        <div class="collapse navbar-collapse" id="navbarNav">
            <ul class="navbar-nav ms-auto gap-3">
                {@render NavItem({
                    url: "/",
                    label: "Dashboard",
                    icon: "bi-speedometer2",
                    active: true,
                })}

                {@render NavItem({
                    url: "/import_transactions",
                    label: "Import Transactions",
                    icon: "bi-cloud-arrow-up",
                })}
            </ul>
        </div>
    </div>
</nav>
