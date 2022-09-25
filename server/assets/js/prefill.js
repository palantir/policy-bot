(function() {
    function prefillSimulation(){
        function prefillParam(param){
            const queryString = window.location.search;
            const urlParams = new URLSearchParams(queryString);
            if (urlParams.has(param)) {
                let paramValue = urlParams.get(param)
                document.getElementById(param).value = paramValue
            }
        }

        const BRANCH = "branch"
        const USERNAME = "username"

        prefillParam(BRANCH)
        prefillParam(USERNAME)
    }
    document.addEventListener('DOMContentLoaded', function() {
        prefillSimulation()
    }, false);


})();