import { makeStyles, Typography } from '@material-ui/core';
import clsx from "clsx";
import React from 'react';
import AnimatedMeshSync from './AnimatedMeshSync';
import AnimatedMeshPattern from "./AnimatedMeshPattern";

const useStyles = makeStyles(() => ({
  loadingWrapper : {
    textAlign : "center",
    marginTop : "calc(50vh - 141px)",
    transform : "translateY(-50%)",
  }
}));

function LoadingScreen({ message, className,animatedIcon, ...other }) {
  const classes = useStyles();

  return (
    <div className={clsx(classes.loadingWrapper, className)} {...other}>
      {animatedIcon === "Pattern" ?  ( <AnimatedMeshPattern style={{ height : "75px" }} />):
        (
          <>
            <AnimatedMeshSync style={{ height : "75px" }} />
            <img
              src="/static/img/meshery-logo/meshery-black.svg"
              alt="mehsery-logo"
              width="125px"
              style={{ margin : "4px 0px 8px" }}
            />
          </>
        )
      }
      <Typography variant="caption" component="div">
        {message}
      </Typography>
    </div>
  );
}

export default LoadingScreen;