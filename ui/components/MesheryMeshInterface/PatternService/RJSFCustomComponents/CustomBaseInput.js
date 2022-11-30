import React from "react";
import { IconButton, InputAdornment, makeStyles, TextField } from "@material-ui/core";
import { EnlargedTextTooltip, EnlargedTextErrorTooltip } from "../EnlargedTextTooltip";
import HelpOutlineIcon from "../../../../assets/icons/HelpOutlineIcon";
import ErrorOutlineIcon from "../../../../assets/icons/ErrorOutlineIcon";

const BaseInput = (props) => {
  const additional = props.schema?.__additional_property; // check if the field is additional
  const name = (additional ? "Value" : props.label) // || props.id?.split('_')[-1].trim()
  const focused = props.options?.focused // true for datetime-local
  const prettifiedName = name || 'Enter a value'
  const style = {
    display : "flex",
    alignItems : "center",
  }

  const useStyles = makeStyles(() => ({
    root : {
      "& .MuiFormLabel-root" : {
        textTransform : "capitalize"
      },


    }
  }));
  const classes = useStyles();
  return (
    <>
      <div key={props.id} style={style}>
        <TextField
          variant={additional ? "standard" : "outlined"}
          size="small"
          className={classes.root}
          focused={focused}
          type={props.options?.inputType}
          key={props.id}
          style={{ width : "100%" }}
          value={additional && props?.value === "New Value" ? "" : props?.value} // remove the default value i.e. New Value for additionalFields
          id={props.id}
          margin="dense"
          error={props.rawErrors?.length > 0}
          onChange={e => props?.onChange(e.target.value === "" ? props.options.emptyValue : e.target.value)}
          label={`${prettifiedName}`}
          InputProps={{
            style : { padding : "0px 0px 0px 0px", },
            endAdornment : (<InputAdornment position="start">
              {props.rawErrors?.length > 0 && (
                <EnlargedTextErrorTooltip title={
                  <div>
                    {props.rawErrors?.map((error, index) => (
                      <div key={index}>{error}</div>
                    ))}
                  </div>

                }>
                  <IconButton component="span" size="small">
                    <ErrorOutlineIcon width="14px" height="14px" fill="#B32700" style={{ verticalAlign : "middle" }} />
                  </IconButton>
                </EnlargedTextErrorTooltip>
              )}
              {props.schema?.description && (
                <EnlargedTextTooltip title={props.schema?.description}>
                  <IconButton component="span" size="small">
                    <HelpOutlineIcon width="14px" height="14px" fill="black" style={{ verticalAlign : "middle" }} />
                  </IconButton>
                </EnlargedTextTooltip>
              )}
            </InputAdornment>),
          }} />
      </div>
    </>
  )
}

export default BaseInput;
