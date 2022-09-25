(function() {
    function addSimulateRedirect(){
        const buttonId = "details-to-simulate"
        function redirectToSimulate(event) {
            const currentPath = location.href
            let simulatePath = currentPath.replace("details", "simulate")
            document.location = simulatePath
        }
        document.getElementById(buttonId).addEventListener("click", redirectToSimulate)
    }

    document.addEventListener('DOMContentLoaded', function() {
        addSimulateRedirect()
    }, false);


})();